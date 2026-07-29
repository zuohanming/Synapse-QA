from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException

from app.core.config import settings
from app.models.task import TaskCreate, TaskView
from app.services.capture_platform_client import CaptureCommandPoller, CapturePlatformClient
from app.services.capture_session_manager import CaptureSessionManager
from app.services.heartbeat_client import HeartbeatClient
from app.services.task_manager import TaskManager


task_manager = TaskManager()
heartbeat_client = HeartbeatClient(task_manager.stats)
capture_session_manager = CaptureSessionManager()
capture_platform_client = CapturePlatformClient()
capture_command_poller = CaptureCommandPoller(
    capture_platform_client,
    capture_session_manager,
    poll_interval=settings.capture_command_poll_interval_seconds,
    on_auth_failure=heartbeat_client.notify_auth_failure,
)


def configure_gui_callbacks(capture_status_handler, auth_failure_handler) -> None:
    """注册由 GUI 自行切回 Tk 主线程的状态和认证回调。"""
    capture_session_manager.set_state_callback(capture_status_handler)
    heartbeat_client.set_auth_failure_handler(auth_failure_handler)


def create_app() -> FastAPI:
    """创建 FastAPI 应用并注册执行器对外接口。"""

    @asynccontextmanager
    async def lifespan(_app: FastAPI):
        heartbeat_client.start()
        capture_command_poller.start()
        try:
            yield
        finally:
            capture_command_poller.stop()
            heartbeat_client.stop()

    app = FastAPI(title=settings.app_name, version="1.0.0", lifespan=lifespan)
    @app.get("/health")
    def health() -> dict:
        return {
            "status": "ok",
            "executorId": settings.executor_id,
            "maxWorkers": settings.max_workers,
            "capture": capture_session_manager.health_state(),
        }

    @app.post("/tasks", response_model=TaskView, status_code=202)
    def submit_task(task: TaskCreate) -> TaskView:
        # 任务提交后立即返回当前视图，实际执行在线程池中异步完成。
        try:
            return task_manager.submit(task)
        except ValueError as error:
            raise HTTPException(status_code=400, detail=str(error)) from error

    @app.get("/tasks", response_model=list[TaskView])
    def list_tasks() -> list[TaskView]:
        return task_manager.list_tasks()

    @app.get("/tasks/{task_id}", response_model=TaskView)
    def get_task(task_id: str) -> TaskView:
        task = task_manager.get(task_id)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")
        return task

    @app.post("/tasks/{task_id}/cancel", response_model=TaskView)
    def cancel_task(task_id: str) -> TaskView:
        # 当前版本只可靠取消尚未开始的任务，运行中任务由具体 Runner 决定是否支持中断。
        task = task_manager.cancel(task_id)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")
        return task

    return app
