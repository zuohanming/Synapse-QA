from concurrent.futures import Future, ThreadPoolExecutor
from datetime import datetime
import logging
from threading import Lock
from uuid import uuid4

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult, TaskStatus, TaskType, TaskView
from app.runners.api_runner import ApiRunner
from app.runners.noop_runner import NoopRunner
from app.runners.playwright_runner import PlaywrightRunner
from app.runners.pytest_runner import PytestRunner
from app.runners.script_runner import ScriptRunner
from app.services.callback_client import notify_callback


logger = logging.getLogger(__name__)


class TaskManager:
    """负责任务登记、状态流转、Runner 调度和执行完成回调。"""

    def __init__(self) -> None:
        # 任务暂存于内存，后续如需跨进程恢复可替换为 Repository/数据库。
        self._executor = ThreadPoolExecutor(max_workers=settings.max_workers)
        self._tasks: dict[str, TaskView] = {}
        self._futures: dict[str, Future] = {}
        self._lock = Lock()
        self._runners = {
            TaskType.noop: NoopRunner(),
            TaskType.script: ScriptRunner(),
            TaskType.api: ApiRunner(),
            TaskType.ui: PlaywrightRunner(),
            TaskType.unit: PytestRunner(),
        }

    def submit(self, task: TaskCreate) -> TaskView:
        """登记任务并提交到线程池执行。"""

        task_id = task.task_id or str(uuid4())
        with self._lock:
            if task_id in self._tasks:
                raise ValueError("任务 ID 已存在")
            active_count = sum(
                1 for item in self._tasks.values()
                if item.status in (TaskStatus.queued, TaskStatus.running)
            )
            if active_count >= max(1, settings.max_workers * 2):
                logger.warning("执行器队列已满：active=%s limit=%s", active_count, max(1, settings.max_workers * 2))
                raise ValueError("执行器队列已满，请稍后重试")
            view = TaskView(
                taskId=task_id,
                type=task.type,
                status=TaskStatus.queued,
                payload=task.payload,
                callbackUrl=task.callback_url,
                createdAt=datetime.now(),
            )
            self._tasks[task_id] = view
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

    def cancel(self, task_id: str) -> TaskView | None:
        """取消排队任务；运行中任务暂不做强制进程终止。"""

        with self._lock:
            task = self._tasks.get(task_id)
            if not task:
                return None
            future = self._futures.get(task_id)
            if task.status == TaskStatus.queued and future and future.cancel():
                task.status = TaskStatus.canceled
                task.finished_at = datetime.now()
                task.result = TaskResult(exitCode=None, error="任务已取消")
            elif task.status == TaskStatus.running:
                task.result = TaskResult(exitCode=None, error="任务正在运行，当前 Runner 不支持强制中断")
            return task

    def _execute(self, task_id: str, task: TaskCreate) -> None:
        """线程池中的执行入口，统一处理成功、失败和回调。"""

        self._mark_running(task_id)
        logger.info("任务开始执行：task_id=%s type=%s", task_id, task.type.value)
        try:
            runner = self._runners[task.type]
            if task.type == TaskType.ui:
                result = runner.run(task, lambda output: self._update_progress(task_id, output))
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

    def _finish(self, task_id: str, status: TaskStatus, result: TaskResult) -> TaskView | None:
        with self._lock:
            task = self._tasks.get(task_id)
            if not task:
                return None
            if task.status == TaskStatus.canceled:
                return task
            task.status = status
            task.result = result
            task.finished_at = datetime.now()
            return task
