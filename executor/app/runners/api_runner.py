import json
import socket
import time
import urllib.error
import urllib.parse
import urllib.request
from ipaddress import ip_address
from typing import Callable

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class ApiRunner(Runner):
    """执行受限 HTTP 调试请求，并实时报告请求与响应阶段。"""

    def run(self, task: TaskCreate, progress: Callable | None = None, canceled: Callable[[], bool] | None = None) -> TaskResult:
        method = str(task.payload.get("method", "GET")).upper()
        url = str(task.payload.get("url") or "")
        if not url:
            raise ValueError("api 任务必须提供 payload.url")
        self._validate_target(url)

        headers = {str(key): str(value) for key, value in (task.payload.get("headers") or {}).items()}
        body = task.payload.get("body")
        timeout = max(1, min(300, int(task.payload.get("timeoutSeconds") or settings.default_timeout_seconds)))
        max_bytes = min(20 << 20, int(task.payload.get("maxResponseBytes") or (20 << 20)))
        data = body.encode("utf-8") if isinstance(body, str) and body else None
        request = urllib.request.Request(url, data=data, headers=headers, method=method)
        started = time.perf_counter()

        if progress:
            progress("request.built", "request", "最终请求已构建", 20, {"method": method, "url": url, "sequence": 2})
            progress("request.connecting", "request", "正在连接目标服务", 35, {"sequence": 3})
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                if progress:
                    progress("response.headers", "response", "已收到响应头", 60, {"statusCode": response.status, "sequence": 4})
                raw = self._read_response(response, max_bytes, canceled)
                if len(raw) > max_bytes:
                    raise ValueError("API_RESPONSE_TOO_LARGE")
                result = self._result(response.status, response.headers, raw, started)
        except urllib.error.HTTPError as error:
            raw = self._read_response(error, max_bytes, canceled)
            if len(raw) > max_bytes:
                raise ValueError("API_RESPONSE_TOO_LARGE")
            if progress:
                progress("response.headers", "response", "已收到响应头", 60, {"statusCode": error.code, "sequence": 4})
            result = self._result(error.code, error.headers, raw, started)

        if progress:
            progress("response.completed", "response", "响应接收完成", 90, {"statusCode": result["statusCode"], "durationMs": result["durationMs"], "sequence": 5})
        return TaskResult(exit_code=0, output=json.dumps(result, ensure_ascii=False))

    @staticmethod
    def _read_response(response, max_bytes: int, canceled: Callable[[], bool] | None) -> bytes:
        chunks: list[bytes] = []
        size = 0
        while True:
            if canceled and canceled():
                raise InterruptedError("任务已取消")
            chunk = response.read(min(64 << 10, max_bytes + 1 - size))
            if not chunk:
                return b"".join(chunks)
            chunks.append(chunk)
            size += len(chunk)
            if size > max_bytes:
                raise ValueError("API_RESPONSE_TOO_LARGE")

    @staticmethod
    def _result(status: int, headers, raw: bytes, started: float) -> dict:
        return {
            "statusCode": status,
            "headers": dict(headers.items()),
            "body": raw[: 1 << 20].decode("utf-8", errors="replace"),
            "responseSize": len(raw),
            "truncated": len(raw) > (1 << 20),
            "durationMs": round((time.perf_counter() - started) * 1000),
        }

    @staticmethod
    def _validate_target(url: str) -> None:
        parsed = urllib.parse.urlparse(url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("仅允许 HTTP/HTTPS 目标")
        for info in socket.getaddrinfo(parsed.hostname, parsed.port or (443 if parsed.scheme == "https" else 80)):
            address = ip_address(info[4][0])
            if address.is_loopback or address.is_link_local or address.is_multicast or address.is_unspecified:
                raise ValueError("API_SSRF_TARGET_REJECTED")
