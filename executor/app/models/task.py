from datetime import datetime
from enum import StrEnum
from typing import Any

from pydantic import BaseModel, Field


class TaskType(StrEnum):
    """后端下发的执行任务类型。"""

    noop = "noop"
    script = "script"
    api = "api"
    ui = "ui"
    unit = "unit"


class TaskStatus(StrEnum):
    """任务在执行器内的生命周期状态。"""

    queued = "queued"
    running = "running"
    success = "success"
    failed = "failed"
    canceled = "canceled"


class TaskCreate(BaseModel):
    """创建任务请求，字段别名保持与后端 JSON 协议一致。"""

    task_id: str | None = Field(default=None, alias="taskId")
    type: TaskType
    # payload 由具体 Runner 解释，执行器只负责透传和基础调度。
    payload: dict[str, Any] = Field(default_factory=dict)
    callback_url: str | None = Field(default=None, alias="callbackUrl")
    event_url: str | None = Field(default=None, alias="eventUrl")

    model_config = {"populate_by_name": True}


class TaskResult(BaseModel):
    """Runner 执行后的标准结果，用于接口返回和后端回调。"""

    exit_code: int | None = Field(default=None, alias="exitCode")
    output: str = ""
    error: str = ""
    artifacts: list[str] = Field(default_factory=list)

    model_config = {"populate_by_name": True}


class TaskView(BaseModel):
    """任务查询视图，记录任务状态、时间和执行结果。"""

    task_id: str = Field(alias="taskId")
    type: TaskType
    status: TaskStatus
    payload: dict[str, Any]
    callback_url: str | None = Field(default=None, alias="callbackUrl")
    event_url: str | None = Field(default=None, alias="eventUrl")
    result: TaskResult | None = None
    created_at: datetime = Field(alias="createdAt")
    started_at: datetime | None = Field(default=None, alias="startedAt")
    finished_at: datetime | None = Field(default=None, alias="finishedAt")

    model_config = {"populate_by_name": True}
