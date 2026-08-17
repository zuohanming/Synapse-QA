from concurrent.futures import Future, ThreadPoolExecutor
from datetime import datetime
import json
import logging
from threading import Event, Lock
import time
from uuid import uuid4

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult, TaskStatus, TaskType, TaskView
from app.runners.api_runner import ApiRunner
from app.runners.api_case_runner import ApiCaseRunner
from app.runners.noop_runner import NoopRunner
from app.runners.perf_runner import PerfRunner
from app.runners.playwright_runner import PlaywrightRunner
from app.runners.pytest_runner import PytestRunner
from app.runners.script_runner import ScriptRunner
from app.services.callback_client import notify_callback, notify_event


logger = logging.getLogger(__name__)


class TaskManager:
    """负责任务登记、状态流转、Runner 调度和执行完成回调。"""

    def __init__(self) -> None:
        # 任务暂存于内存，后续如需跨进程恢复可替换为 Repository/数据库。
        self._executor = ThreadPoolExecutor(max_workers=settings.max_workers)
        self._tasks: dict[str, TaskView] = {}
        self._futures: dict[str, Future] = {}
        self._cancel_events: dict[str, Event] = {}
        # 已取消 task_id 墓碑：task_id -> 过期时间戳（monotonic），用于幂等返回已取消（SPEC §3.3）。
        self._canceled_tombstones: dict[str, float] = {}
        self._lock = Lock()
        self._runners = {
            TaskType.noop: NoopRunner(),
            TaskType.script: ScriptRunner(),
            TaskType.api: ApiRunner(),
            TaskType.api_case: ApiCaseRunner(),
            TaskType.ui: PlaywrightRunner(),
            TaskType.unit: PytestRunner(),
            TaskType.perf: PerfRunner(),
        }

    def submit(self, task: TaskCreate) -> TaskView:
        """登记任务并提交到线程池执行；对已存在/已取消的 task_id 幂等返回现有视图。"""

        task_id = task.task_id or str(uuid4())
        with self._lock:
            self._prune_tombstones()
            if task_id in self._canceled_tombstones:
                logger.info("收到已取消任务的重复提交：task_id=%s，幂等返回已取消", task_id)
                return self._canceled_view(task_id, task)
            if task_id in self._tasks:
                logger.info("收到已存在任务的重复提交：task_id=%s，幂等返回现有视图", task_id)
                return self._tasks[task_id]
            active_count = sum(
                1 for item in self._tasks.values()
                if item.status in (TaskStatus.queued, TaskStatus.running)
            )
            if active_count >= max(1, settings.max_workers * 2):
                logger.warning("执行器队列已满：active=%s limit=%s", active_count, max(1, settings.max_workers * 2))
                raise ValueError("执行器队列已满，请稍后重试")
            if task.type == TaskType.perf:
                active_perf = sum(
                    1 for item in self._tasks.values()
                    if item.type == TaskType.perf and item.status in (TaskStatus.queued, TaskStatus.running)
                )
                if active_perf >= settings.perf_max_concurrent:
                    logger.warning("性能测试并发槽位已满：active=%s limit=%s", active_perf, settings.perf_max_concurrent)
                    raise ValueError("性能测试并发槽位已满，请稍后重试")
            view = TaskView(
                taskId=task_id,
                type=task.type,
                status=TaskStatus.queued,
                payload=task.payload,
                callbackUrl=task.callback_url,
                eventUrl=task.event_url,
                callbackToken=task.callback_token,
                createdAt=datetime.now(),
            )
            self._tasks[task_id] = view
            self._cancel_events[task_id] = Event()
            logger.info("收到任务：task_id=%s type=%s", task_id, task.type.value)
            future = self._executor.submit(self._execute, task_id, task)
            self._futures[task_id] = future
            logger.info("任务已进入队列：task_id=%s", task_id)
            return view

    def list_tasks(self) -> list[TaskView]:
        with self._lock:
            return sorted(self._tasks.values(), key=lambda item: item.created_at, reverse=True)

    def get(self, task_id: str) -> TaskView | None:
        with self._lock:
            return self._tasks.get(task_id)

    def stats(self) -> dict[str, int]:
        """返回心跳上报所需的任务负载统计。"""

        with self._lock:
            queued = sum(1 for task in self._tasks.values() if task.status == TaskStatus.queued)
            running = sum(1 for task in self._tasks.values() if task.status == TaskStatus.running)
            return {"queuedTasks": queued, "runningTasks": running}

    def cancel(self, task_id: str) -> TaskView:
        """取消任务；对不存在/已取消的任务记墓碑并幂等返回已取消（SPEC §3.3）。"""

        callback_view = None
        with self._lock:
            self._prune_tombstones()
            task = self._tasks.get(task_id)
            if not task:
                # 任务不存在：记墓碑，后续创建/启动请求幂等返回已取消（补偿取消）。
                self._canceled_tombstones[task_id] = time.monotonic() + settings.perf_cancel_tombstone_ttl_seconds
                logger.info("收到不存在任务的取消请求，记录墓碑：task_id=%s", task_id)
                return self._canceled_view(task_id, TaskCreate(taskId=task_id, type=TaskType.perf, payload={}))
            future = self._futures.get(task_id)
            if task.status == TaskStatus.queued and future and future.cancel():
                task.status = TaskStatus.canceled
                task.finished_at = datetime.now()
                task.result = TaskResult(exitCode=None, error="任务已取消")
                if task.type == TaskType.perf:
                    callback_view = task
            elif task.status == TaskStatus.running:
                cancel_event = self._cancel_events.get(task_id)
                if cancel_event:
                    cancel_event.set()
                task.status = TaskStatus.canceled
                task.finished_at = datetime.now()
                task.result = TaskResult(exitCode=None, error="任务取消请求已生效")
            # 记墓碑：取消后的重复提交/启动请求幂等返回已取消。
            self._canceled_tombstones[task_id] = time.monotonic() + settings.perf_cancel_tombstone_ttl_seconds
        if callback_view:
            notify_callback(callback_view)
        return task

    def exists_or_canceled(self, task_id: str) -> bool:
        """判断 task_id 是否已登记或命中取消墓碑，供路由区分幂等命中与新建。"""

        with self._lock:
            self._prune_tombstones()
            return task_id in self._tasks or task_id in self._canceled_tombstones

    def _canceled_view(self, task_id: str, task: TaskCreate) -> TaskView:
        now = datetime.now()
        return TaskView(
            taskId=task_id,
            type=task.type,
            status=TaskStatus.canceled,
            payload=task.payload,
            callbackUrl=task.callback_url,
            eventUrl=task.event_url,
            callbackToken=task.callback_token,
            result=TaskResult(exitCode=None, error="任务已取消"),
            createdAt=now,
            finishedAt=now,
        )

    def _prune_tombstones(self) -> None:
        now = time.monotonic()
        expired = [task_id for task_id, expire in self._canceled_tombstones.items() if expire <= now]
        for task_id in expired:
            del self._canceled_tombstones[task_id]

    def _execute(self, task_id: str, task: TaskCreate) -> None:
        """线程池中的执行入口，统一处理成功、失败和回调。"""

        self._mark_running(task_id)
        logger.info("任务开始执行：task_id=%s type=%s", task_id, task.type.value)
        try:
            runner = self._runners[task.type]
            if task.type == TaskType.ui:
                result = runner.run(task, lambda output: self._update_progress(task_id, output))
            elif task.type in (TaskType.api, TaskType.api_case):
                cancel_event = self._cancel_events[task_id]
                result = runner.run(
                    task,
                    lambda event_type, stage, message, progress, data=None: self._notify_api_event(task_id, event_type, stage, message, progress, data),
                    cancel_event.is_set,
                )
            elif task.type == TaskType.perf:
                cancel_event = self._cancel_events[task_id]
                result = runner.run(
                    task,
                    lambda output: self._update_progress(task_id, output),
                    cancel_event.is_set,
                    lambda metadata: self._notify_perf_started(task_id, metadata),
                )
            else:
                result = runner.run(task)
            status = TaskStatus.success if result.exit_code == 0 else TaskStatus.failed
        except Exception as error:
            logger.exception("任务执行异常：task_id=%s", task_id)
            result = TaskResult(exitCode=1, output=self._progress_output(task_id), error=str(error))
            status = TaskStatus.failed
        final_view = self._finish(task_id, status, result)
        if final_view:
            if status == TaskStatus.success:
                logger.info("任务执行成功：task_id=%s", task_id)
            else:
                logger.error("任务执行失败：task_id=%s error=%s", task_id, result.error or "未知错误")
            notify_callback(final_view)

    def _mark_running(self, task_id: str) -> None:
        with self._lock:
            task = self._tasks[task_id]
            if task.status != TaskStatus.canceled:
                task.status = TaskStatus.running
                task.started_at = datetime.now()

    def _notify_perf_started(self, task_id: str, metadata: dict) -> None:
        with self._lock:
            task = self._tasks.get(task_id)
            if not task or task.status != TaskStatus.running:
                return
            task.result = TaskResult(exitCode=None, output=json.dumps(metadata, ensure_ascii=False))
        notify_callback(task)

    def _update_progress(self, task_id: str, output: str) -> None:
        with self._lock:
            task = self._tasks.get(task_id)
            if task and task.status == TaskStatus.running:
                task.result = TaskResult(exitCode=None, output=output)
        latest_line = output.rsplit("\n", 1)[-1].strip()
        if latest_line:
            logger.info("任务步骤：task_id=%s %s", task_id, latest_line)

    def _progress_output(self, task_id: str) -> str:
        with self._lock:
            task = self._tasks.get(task_id)
            return task.result.output if task and task.result else ""

    def _notify_api_event(self, task_id: str, event_type: str, stage: str, message: str, progress: int, data: dict | None = None) -> None:
        with self._lock:
            task = self._tasks.get(task_id)
            if not task:
                return
            sequence = int((data or {}).pop("sequence", 0)) or max(2, progress)
        logger.info("API 调试步骤：task_id=%s type=%s message=%s", task_id, event_type, message)
        notify_event(task, sequence, event_type, stage, message, progress, data)

    def _finish(self, task_id: str, status: TaskStatus, result: TaskResult) -> TaskView | None:
        with self._lock:
            task = self._tasks.get(task_id)
            if not task:
                return None
            if task.status == TaskStatus.canceled:
                # 取消后 runner 仍可能产出部分结果（如 perf 的本地采样兜底），
                # 此时用部分结果覆盖 cancel() 写入的占位 result，但保留 canceled 状态。
                if result and result.output:
                    task.result = result
                return task
            task.status = status
            task.result = result
            task.finished_at = datetime.now()
            return task
