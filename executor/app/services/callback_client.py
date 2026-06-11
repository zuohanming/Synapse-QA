import json
import urllib.request

from app.models.task import TaskView


def notify_callback(task: TaskView) -> None:
    """将任务最终状态回调给后端，回调失败不改变本地任务结果。"""

    if not task.callback_url:
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
    except Exception:
        # 回调失败不能反向改变本地任务结果，后续可扩展重试队列。
        return
