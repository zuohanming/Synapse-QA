import asyncio
import json
import logging
import threading
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Awaitable, Callable

from app.core.config import settings
from app.models.capture import CaptureCommand, CaptureMode, CaptureStartCommand
from app.services.capture_session_manager import (
    CaptureHeartbeatContext,
    CaptureSessionConflict,
    CaptureSessionManager,
    CaptureStartupError,
)


logger = logging.getLogger(__name__)
_MAX_RESPONSE_BYTES = 1024 * 1024


@dataclass(frozen=True)
class PlatformResponse:
    status: int
    data: Any


Transport = Callable[[str, str, dict[str, str], dict[str, Any] | None], Awaitable[PlatformResponse]]


class CapturePlatformClient:
    """页面元素采集平台协议客户端；日志中不包含 token、receipt 或响应正文。"""

    def __init__(
        self,
        *,
        base_url: str | None = None,
        executor_id: str | None = None,
        executor_token: str | None = None,
        transport: Transport | None = None,
    ) -> None:
        self._base_url = (base_url if base_url is not None else settings.platform_base_url).rstrip("/")
        self._executor_id = executor_id if executor_id is not None else settings.executor_id
        self._executor_token = executor_token
        self._transport = transport or self._urllib_transport

    def _long_token(self) -> str:
        return self._executor_token if self._executor_token is not None else settings.executor_shared_token

    async def claim_commands(self) -> PlatformResponse:
        query = urllib.parse.urlencode({"executorId": self._executor_id})
        response = await self._request(
            "GET",
            f"/api/executor/element-capture/commands?{query}",
            {"X-Executor-ID": self._executor_id, "X-Executor-Token": self._long_token()},
            None,
        )
        return _unwrap_response(response)

    async def ack(self, command_id: int, receipt: str) -> PlatformResponse:
        response = await self._request(
            "POST",
            f"/api/executor/element-capture/commands/{command_id}/ack?{urllib.parse.urlencode({'executorId': self._executor_id})}",
            {"X-Executor-ID": self._executor_id, "X-Executor-Token": self._long_token()},
            {"receipt": receipt},
        )
        return _unwrap_response(response)

    async def heartbeat(self, context: CaptureHeartbeatContext) -> PlatformResponse:
        response = await self._session_request(
            "POST",
            f"/api/executor/element-capture/{urllib.parse.quote(context.session_id, safe='')}/heartbeat",
            context,
            {
                "executorId": self._executor_id,
                "browserContextId": context.browser_context_id,
                "currentUrl": context.current_url,
                "commandReceipt": context.command_receipt,
            },
        )
        return _unwrap_response(response)

    async def add_candidate(self, context: CaptureHeartbeatContext, payload: dict[str, Any]) -> PlatformResponse:
        body = dict(payload)
        body["executorId"] = self._executor_id
        response = await self._session_request(
            "POST",
            f"/api/executor/element-capture/{urllib.parse.quote(context.session_id, safe='')}/candidates",
            context,
            body,
        )
        return _unwrap_response(response)

    async def fail(self, context: CaptureHeartbeatContext, reason: str) -> PlatformResponse:
        response = await self._session_request(
            "POST",
            f"/api/executor/element-capture/{urllib.parse.quote(context.session_id, safe='')}/fail",
            context,
            {"executorId": self._executor_id, "reason": reason},
        )
        return _unwrap_response(response)

    async def _session_request(
        self,
        method: str,
        path: str,
        context: CaptureHeartbeatContext,
        payload: dict[str, Any],
    ) -> PlatformResponse:
        return await self._request(
            method,
            path,
            {
                "X-Executor-ID": self._executor_id,
                "Authorization": f"Bearer {context.token}",
            },
            payload,
        )

    async def _request(
        self,
        method: str,
        path: str,
        headers: dict[str, str],
        payload: dict[str, Any] | None,
    ) -> PlatformResponse:
        if not self._base_url:
            return PlatformResponse(0, None)
        try:
            return await self._transport(method, self._base_url + path, headers, payload)
        except (OSError, TimeoutError, urllib.error.URLError):
            logger.warning("采集平台网络请求失败：method=%s path=%s", method, path.split("?", 1)[0])
            return PlatformResponse(0, None)

    async def _urllib_transport(
        self,
        method: str,
        url: str,
        headers: dict[str, str],
        payload: dict[str, Any] | None,
    ) -> PlatformResponse:
        return await asyncio.to_thread(_send_urllib_request, method, url, headers, payload)


class CaptureCommandPoller:
    """在单一 asyncio 事件循环中轮询命令并驱动 Playwright 会话。"""

    def __init__(
        self,
        client: CapturePlatformClient,
        manager: CaptureSessionManager,
        *,
        poll_interval: float = 2,
        on_auth_failure: Callable[[], None] | None = None,
    ) -> None:
        self._client = client
        self._manager = manager
        self._poll_interval = poll_interval
        self._on_auth_failure = on_auth_failure
        self._stop_event = threading.Event()
        self._stopped_event = threading.Event()
        self._stopped_event.set()
        self._thread_lock = threading.Lock()
        self._thread: threading.Thread | None = None
        self._sleep = asyncio.sleep

    def set_auth_failure_handler(self, handler: Callable[[], None] | None) -> None:
        self._on_auth_failure = handler

    def start(self) -> None:
        if not settings.platform_base_url:
            return
        with self._thread_lock:
            if self._thread and self._thread.is_alive():
                return
            self._stop_event.clear()
            self._stopped_event.clear()
            self._thread = threading.Thread(target=self._thread_main, name="capture-command-poller", daemon=True)
            self._thread.start()

    def request_stop(self) -> None:
        self._stop_event.set()

    def stop(self) -> None:
        self.request_stop()
        with self._thread_lock:
            thread = self._thread
        if thread and thread is not threading.current_thread():
            thread.join(timeout=max(5, self._poll_interval + 2))
            self._stopped_event.wait(timeout=1)

    def _thread_main(self) -> None:
        try:
            asyncio.run(self.run_forever())
        except Exception:
            logger.exception("采集命令轮询器异常退出")
        finally:
            self._stopped_event.set()

    async def run_forever(self) -> None:
        try:
            while not self._stop_event.is_set():
                await self.run_once()
                await self._sleep(self._poll_interval)
        finally:
            await self._manager.close()

    async def run_once(self) -> None:
        response = await self._client.claim_commands()
        if response.status == 401:
            await self._terminate_active("unauthorized")
            self._notify_auth_failure()
            return
        if response.status == 200:
            for raw_command in response.data or []:
                await self._handle_command(raw_command)
        await self._send_heartbeat()
        await self._flush_candidates()

    async def _handle_command(self, raw_command: Any) -> None:
        try:
            command = raw_command if isinstance(raw_command, CaptureCommand) else CaptureCommand.model_validate(raw_command)
        except Exception:
            logger.warning("忽略结构无效的采集命令")
            return
        if command.type == "start":
            try:
                start_command = (
                    raw_command
                    if isinstance(raw_command, CaptureStartCommand)
                    else CaptureStartCommand.model_validate(
                        raw_command.model_dump(by_alias=True, mode="json")
                        if isinstance(raw_command, CaptureCommand)
                        else raw_command
                    )
                )
                await self._manager.start(start_command)
            except (ValueError, CaptureStartupError, CaptureSessionConflict) as error:
                logger.warning("采集 start 命令执行失败：error_type=%s", type(error).__name__)
                if command.token:
                    response = await self._client.fail(
                        CaptureHeartbeatContext(command.session_id, command.token, "", "", command.receipt),
                        "browser_start_failed",
                    )
                    await self._handle_terminal_response(response, command.session_id)
            return

        try:
            if command.type == "set_mode":
                if command.mode is None:
                    raise ValueError("模式命令缺少 mode")
                await self._manager.set_mode(command.session_id, CaptureMode(command.mode))
            else:
                await self._manager.stop(command.session_id, command.type)
        except (ValueError, CaptureSessionConflict):
            logger.warning("采集控制命令未应用：type=%s", command.type)
            return

        response = await self._client.ack(command.id, command.receipt)
        await self._handle_terminal_response(response, command.session_id)

    async def _send_heartbeat(self) -> None:
        context = self._manager.heartbeat_context()
        if not context:
            return
        response = await self._client.heartbeat(context)
        if 200 <= response.status < 300:
            self._manager.confirm_start_heartbeat()
            return
        await self._handle_terminal_response(response, context.session_id)

    async def _flush_candidates(self) -> None:
        for pending in self._manager.pending_candidates():
            context = self._manager.heartbeat_context()
            if not context:
                return
            response = await self._client.add_candidate(context, pending.payload)
            if 200 <= response.status < 300:
                await self._manager.mark_candidate_sent(pending.capture_id)
            elif response.status in {401, 409}:
                await self._handle_terminal_response(response, context.session_id)
                return
            elif 400 <= response.status < 500:
                await self._manager.discard_candidate(pending.capture_id)
            else:
                return

    async def _handle_terminal_response(self, response: PlatformResponse, session_id: str) -> None:
        if response.status not in {401, 409}:
            return
        try:
            if response.status == 401:
                await self._terminate_active("unauthorized")
            else:
                await self._manager.stop(session_id, "conflict")
        except CaptureSessionConflict:
            pass
        if response.status == 401:
            self._notify_auth_failure()

    async def _terminate_active(self, reason: str) -> None:
        context = self._manager.heartbeat_context()
        if context:
            await self._manager.stop(context.session_id, reason)

    def _notify_auth_failure(self) -> None:
        if self._on_auth_failure:
            try:
                self._on_auth_failure()
            except Exception:
                logger.exception("采集认证失效回调失败")


def _unwrap_response(response: PlatformResponse) -> PlatformResponse:
    if isinstance(response.data, dict) and ("data" in response.data or "error" in response.data):
        return PlatformResponse(response.status, response.data.get("data"))
    return response


def _send_urllib_request(
    method: str,
    url: str,
    headers: dict[str, str],
    payload: dict[str, Any] | None,
) -> PlatformResponse:
    body = None if payload is None else json.dumps(payload, ensure_ascii=False).encode("utf-8")
    request_headers = dict(headers)
    if body is not None:
        request_headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=body, headers=request_headers, method=method)
    opener = urllib.request.build_opener(_NoRedirectHandler())
    try:
        with opener.open(request, timeout=5) as response:
            raw = response.read(_MAX_RESPONSE_BYTES + 1)
            if len(raw) > _MAX_RESPONSE_BYTES:
                return PlatformResponse(0, None)
            return PlatformResponse(response.status, _decode_json(raw))
    except urllib.error.HTTPError as error:
        try:
            # 错误正文可能包含平台内部信息，状态机只需要状态码。
            return PlatformResponse(error.code, None)
        finally:
            error.close()


class _NoRedirectHandler(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        del req, fp, code, msg, headers, newurl
        return None


def _decode_json(raw: bytes) -> Any:
    if not raw:
        return None
    try:
        return json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return None
