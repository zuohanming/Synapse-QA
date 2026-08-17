import json
import logging
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from importlib.util import find_spec
from typing import Callable

from app.core.config import settings


logger = logging.getLogger(__name__)


class _NoRedirectHandler(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        del req, fp, code, msg, headers, newurl
        return None


class HeartbeatClient:
    """负责执行器向平台注册和定时心跳。"""

    def __init__(self, stats_provider: Callable[[], dict[str, int]], on_auth_failure: Callable[[], None] | None = None) -> None:
        self._stats_provider = stats_provider
        self._stop_event = threading.Event()
        self._thread: threading.Thread | None = None
        self._on_auth_failure = on_auth_failure
        self._auth_failure_notified = threading.Event()
        self._auth_failure_lock = threading.Lock()

    def set_auth_failure_handler(self, handler: Callable[[], None] | None) -> None:
        self._on_auth_failure = handler

    def authenticate(self, token: str) -> tuple[bool, str]:
        payload = {
            "executorId": settings.executor_id,
            "name": settings.executor_name,
            "endpoint": settings.executor_endpoint,
            "version": settings.executor_version,
            "maxWorkers": settings.max_workers,
            "supportedTypes": settings.supported_types,
        }
        status = self._post("/api/executors/register", payload, token, notify_auth_failure=False)
        if status == 200:
            self._auth_failure_notified.clear()
            if token:
                settings.executor_shared_token = token
            # 连接成功后确保心跳循环重新运行：既覆盖「之前 401 已停止循环」的情况，
            # 也覆盖「首次连接」的情况；若循环仍在运行，start() 会自动跳过重复启动。
            self.start()
            return True, "连接成功"
        if status == 401:
            return False, "Token 无效或已失效"
        return False, "无法连接平台，请检查平台地址和网络"

    def start(self) -> None:
        if not settings.platform_base_url:
            return
        if self._thread and self._thread.is_alive():
            return
        self._stop_event.clear()
        self._auth_failure_notified.clear()
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

    def _heartbeat(self, status: str = "online") -> None:
        stats = self._stats_provider()
        payload = {
            "executorId": settings.executor_id,
            "status": status,
            "version": settings.executor_version,
            "maxWorkers": settings.max_workers,
            "runningTasks": stats["runningTasks"],
            "queuedTasks": stats["queuedTasks"],
            "supportedTypes": settings.supported_types,
            "checks": self._checks(),
            "timestamp": datetime.now(timezone.utc).isoformat(),
        }
        self._post("/api/executors/heartbeat", payload)

    def report_stopped(self) -> None:
        """在本地服务停止前主动通知平台。"""
        self._heartbeat("offline")

    def _post(self, path: str, payload: dict, token: str | None = None, notify_auth_failure: bool = True) -> int:
        url = settings.platform_base_url.rstrip("/") + path
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        request = urllib.request.Request(
            url,
            data=data,
            headers={
                "Content-Type": "application/json",
                "X-Executor-Token": token or settings.executor_shared_token,
            },
            method="POST",
        )
        opener = urllib.request.build_opener(_NoRedirectHandler())
        try:
            with opener.open(request, timeout=5) as response:
                return response.status
        except urllib.error.HTTPError as error:
            try:
                logger.warning("执行器心跳鉴权失败：path=%s status=%s", path, error.code)
                if error.code == 401 and notify_auth_failure:
                    self._stop_event.set()
                    self.notify_auth_failure()
                return error.code
            finally:
                error.close()
        except (urllib.error.URLError, TimeoutError):
            logger.warning("执行器无法连接平台：path=%s", path)
            # 平台短暂不可达时不影响本地任务执行，下一轮心跳会继续补报。
            return 0

    def notify_auth_failure(self) -> None:
        """合并普通心跳和采集回调的 401，确保 GUI 只收到一次失效通知。"""
        with self._auth_failure_lock:
            if self._auth_failure_notified.is_set():
                return
            self._auth_failure_notified.set()
        if self._on_auth_failure:
            self._on_auth_failure()

    def _checks(self) -> dict[str, bool]:
        return {
            "pytest": find_spec("pytest") is not None,
            "playwright": find_spec("playwright") is not None,
            "browser": find_spec("playwright") is not None,
        }
