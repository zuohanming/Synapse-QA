import json
import urllib.error
import urllib.request

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class ApiRunner(Runner):
    """发起 HTTP 请求，用于接口级用例或外部系统探活。"""

    def run(self, task: TaskCreate) -> TaskResult:
        # payload.url 是 API 任务的最小必填字段，其余字段按需提供。
        method = str(task.payload.get("method", "GET")).upper()
        url = task.payload.get("url")
        if not url:
            raise ValueError("api 任务必须提供 payload.url")

        headers = task.payload.get("headers") or {}
        body = task.payload.get("body")
        timeout = int(task.payload.get("timeoutSeconds") or settings.default_timeout_seconds)
        data = None
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers = {"Content-Type": "application/json", **headers}

        request = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                output = response.read().decode("utf-8", errors="replace")
                return TaskResult(exit_code=0, output=output)
        except urllib.error.HTTPError as error:
            output = error.read().decode("utf-8", errors="replace")
            return TaskResult(exit_code=error.code, output=output, error=f"HTTP {error.code}")
