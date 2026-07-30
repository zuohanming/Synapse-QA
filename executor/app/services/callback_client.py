import json
import logging
import urllib.request

from app.models.task import TaskView


logger = logging.getLogger(__name__)


def notify_callback(task: TaskView) -> None:
    """将任务最终状态回调给后端，回调失败不改变本地任务结果。"""

    if not task.callback_url:
        logger.info("任务无需回调：task_id=%s", task.task_id)
        return
    payload = task.model_dump(by_alias=True, mode="json")
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    request = urllib.request.Request(
        task.callback_url,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        urllib.request.urlopen(request, timeout=5).close()
        logger.info("任务回调成功：task_id=%s url=%s", task.task_id, task.callback_url)
    except Exception as error:
        logger.error("任务回调失败：task_id=%s url=%s error=%s", task.task_id, task.callback_url, error)


def notify_event(task: TaskView, sequence: int, event_type: str, stage: str, message: str, progress: int, data: dict | None = None) -> None:
    """实时上报 API 调试步骤；单次失败只记录日志，由最终回调兜底。"""

    if not task.event_url:
        return
    payload = {
        "taskId": task.task_id,
        "sequence": sequence,
        "type": event_type,
        "stage": stage,
        "status": "running",
        "message": message,
        "progress": progress,
        "data": data or {},
    }
    request = urllib.request.Request(
        task.event_url,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        urllib.request.urlopen(request, timeout=3).close()
    except Exception as error:
        logger.warning("任务事件回调失败：task_id=%s type=%s error=%s", task.task_id, event_type, error)
