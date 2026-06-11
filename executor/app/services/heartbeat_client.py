import json
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from importlib.util import find_spec
from typing import Callable

from app.core.config import settings


class HeartbeatClient:
    """负责执行器向平台注册和定时心跳。"""

    def __init__(self, stats_provider: Callable[[], dict[str, int]]) -> None:
        self._stats_provider = stats_provider
        self._stop_event = threading.Event()
        self._thread: threading.Thread | None = None

    def start(self) -> None:
        if not settings.platform_base_url:
            return
        if self._thread and self._thread.is_alive():
            return
        self._thread = threading.Thread(target=self._loop, name="executor-heartbeat", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop_event.set()
        if self._thread:
            self._thread.join(timeout=3)

    def _loop(self) -> None:
        self._register()
        while not self._stop_event.wait(settings.heartbeat_interval_seconds):
            self._heartbeat()

    def _register(self) -> None:
        payload = {
            "executorId": settings.executor_id,
            "name": settings.executor_name,
            "endpoint": settings.executor_endpoint,
            "version": settings.executor_version,
            "maxWorkers": settings.max_workers,
            "supportedTypes": settings.supported_types,
        }
        self._post("/api/executors/register", payload)

    def _heartbeat(self) -> None:
        stats = self._stats_provider()
        payload = {
            "executorId": settings.executor_id,
            "status": "online",
            "version": settings.executor_version,
            "maxWorkers": settings.max_workers,
            "runningTasks": stats["runningTasks"],
            "queuedTasks": stats["queuedTasks"],
            "supportedTypes": settings.supported_types,
            "checks": self._checks(),
            "timestamp": datetime.now(timezone.utc).isoformat(),
        }
        self._post("/api/executors/heartbeat", payload)

    def _post(self, path: str, payload: dict) -> None:
        url = settings.platform_base_url.rstrip("/") + path
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        request = urllib.request.Request(
            url,
            data=data,
            headers={
                "Content-Type": "application/json",
                "X-Executor-Token": settings.executor_shared_token,
            },
            method="POST",
        )
        try:
            urllib.request.urlopen(request, timeout=5).close()
        except (urllib.error.URLError, TimeoutError):
            # 平台短暂不可达时不影响本地任务执行，下一轮心跳会继续补报。
            return

    def _checks(self) -> dict[str, bool]:
        return {
            "pytest": find_spec("pytest") is not None,
            "playwright": find_spec("playwright") is not None,
            "browser": find_spec("playwright") is not None,
        }
