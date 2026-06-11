from fastapi import FastAPI, HTTPException

from app.core.config import settings
from app.models.task import TaskCreate, TaskView
from app.services.heartbeat_client import HeartbeatClient
from app.services.task_manager import TaskManager


task_manager = TaskManager()
heartbeat_client = HeartbeatClient(task_manager.stats)


def create_app() -> FastAPI:
    """创建 FastAPI 应用并注册执行器对外接口。"""

    app = FastAPI(title=settings.app_name, version="1.0.0")

    @app.on_event("startup")
    def start_heartbeat() -> None:
        heartbeat_client.start()

    @app.on_event("shutdown")
    def stop_heartbeat() -> None:
        heartbeat_client.stop()

    @app.get("/health")
    def health() -> dict:
        return {
            "status": "ok",
            "executorId": settings.executor_id,
            "maxWorkers": settings.max_workers,
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
