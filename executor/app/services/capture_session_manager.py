import asyncio
import logging
import re
from dataclasses import dataclass, field
from typing import Any, Callable
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
from uuid import uuid4

from app.models.capture import CaptureMode, CaptureStartCommand, CaptureState
from app.services.element_picker import ElementPicker, LocalCapture, PickerSecurityError, scan_for_sensitive_data


logger = logging.getLogger(__name__)

_ALLOWED_CHANNELS = {"chrome", "msedge"}
_SENSITIVE_QUERY_KEYS = {
    "password", "passwd", "token", "access_token", "refresh_token", "api_key", "apikey",
    "session", "cookie", "authorization", "secret", "client_secret",
}


class CaptureSessionConflict(ValueError):
    pass


class CaptureStartupError(RuntimeError):
    pass


@dataclass(frozen=True)
class CaptureHeartbeatContext:
    session_id: str
    token: str = field(repr=False)
    browser_context_id: str
    current_url: str
    command_receipt: str = field(default="", repr=False)


@dataclass
class PendingCandidate:
    capture: LocalCapture

    @property
    def capture_id(self) -> str:
        return self.capture.capture_id

    @property
    def payload(self) -> dict[str, object]:
        return self.capture.candidate.platform_payload()


@dataclass
class _ActiveSession:
    state: CaptureState
    token: str = field(repr=False)
    receipt: str = field(repr=False)
    navigation_url: str = field(repr=False)
    current_url: str
    lease: Any
    browser: Any
    context: Any
    page: Any
    pending: dict[str, PendingCandidate] = field(default_factory=dict)


class PlaywrightBrowserLease:
    def __init__(self, playwright: Any, browser: Any) -> None:
        self.playwright = playwright
        self.browser = browser

    async def close(self) -> None:
        try:
            await self.browser.close()
        finally:
            await self.playwright.stop()


class PlaywrightBrowserFactory:
    async def launch(self, *, channel: str, headless: bool) -> PlaywrightBrowserLease:
        from playwright.async_api import async_playwright

        playwright = await async_playwright().start()
        try:
            browser = await playwright.chromium.launch(channel=channel, headless=headless)
        except Exception:
            await playwright.stop()
            raise
        return PlaywrightBrowserLease(playwright, browser)


class CaptureSessionManager:
    """在独立 Playwright context 中管理唯一页面元素采集会话。"""

    def __init__(
        self,
        browser_factory: Any | None = None,
        picker: ElementPicker | None = None,
        state_callback: Callable[[bool, str], None] | None = None,
    ) -> None:
        self._browser_factory = browser_factory or PlaywrightBrowserFactory()
        self._picker = picker or ElementPicker()
        self._state_callback = state_callback
        self._session: _ActiveSession | None = None
        self._lock = asyncio.Lock()

    def set_state_callback(self, callback: Callable[[bool, str], None] | None) -> None:
        self._state_callback = callback

    async def start(self, command: CaptureStartCommand) -> CaptureState:
        if command.headless:
            raise ValueError("页面元素采集仅支持有头浏览器")
        if command.browser_channel not in _ALLOWED_CHANNELS:
            raise ValueError("浏览器通道仅允许 chrome 或 msedge")
        _validate_navigation_url(command.url)
        async with self._lock:
            if self._session:
                if self._session.state.session_id == command.session_id:
                    # start 租约重投会轮换一次性 token 和 receipt；浏览器资源保持幂等。
                    self._session.token = command.token
                    self._session.receipt = command.receipt
                    return self._session.state
                raise CaptureSessionConflict("已有页面元素采集会话正在运行")

            lease = browser = context = page = None
            try:
                lease = await self._browser_factory.launch(channel=command.browser_channel, headless=False)
                browser = lease.browser
                context = await browser.new_context()
                page = await context.new_page()
                browser_context_id = str(uuid4())
                state = CaptureState(
                    sessionId=command.session_id,
                    browserContextId=browser_context_id,
                    mode=command.mode,
                    pageTitle="",
                )
                self._session = _ActiveSession(
                    state=state,
                    token=command.token,
                    receipt=command.receipt,
                    navigation_url=command.url,
                    current_url=_sanitize_current_url(command.url),
                    lease=lease,
                    browser=browser,
                    context=context,
                    page=page,
                )

                async def receive_pick(source: dict[str, Any], handle: Any, payload: dict[str, Any]) -> None:
                    await self._handle_pick(command.session_id, source, handle, payload)

                await self._picker.install(context, page, receive_pick)
                await page.goto(command.url, wait_until="domcontentloaded")
                title = _safe_page_title(await page.title())
                self._session.current_url = _sanitize_current_url(page.url)
                self._session.state.page_title = title
                self._notify_state(True, title)
                return self._session.state
            except Exception as error:
                failed = self._session
                self._session = None
                if failed:
                    await self._cleanup_session(failed)
                else:
                    await self._cleanup_resources(page, context, lease)
                    await self._picker.close()
                logger.error("浏览器采集启动失败：error_type=%s", type(error).__name__)
                raise CaptureStartupError("浏览器采集启动失败") from error

    async def set_mode(self, session_id: str, mode: CaptureMode) -> None:
        async with self._lock:
            session = self._require_session(session_id)
            if session.state.mode == mode:
                return
            await self._picker.set_mode(session.page, mode)
            session.state.mode = mode

    async def stop(self, session_id: str, reason: str) -> None:
        del reason
        async with self._lock:
            if not self._session:
                return
            if self._session.state.session_id != session_id:
                raise CaptureSessionConflict("采集会话与当前本地会话不一致")
            session = self._session
            self._session = None
            await self._cleanup_session(session)
            self._notify_state(False, "")

    async def close(self) -> None:
        async with self._lock:
            session = self._session
            self._session = None
            if session:
                await self._cleanup_session(session)
                self._notify_state(False, "")
            else:
                await self._picker.close()

    def heartbeat_context(self) -> CaptureHeartbeatContext | None:
        session = self._session
        if not session:
            return None
        try:
            session.current_url = _sanitize_current_url(session.page.url)
        except (ValueError, AttributeError):
            pass
        return CaptureHeartbeatContext(
            session_id=session.state.session_id,
            token=session.token,
            browser_context_id=session.state.browser_context_id,
            current_url=session.current_url,
            command_receipt=session.receipt,
        )

    def confirm_start_heartbeat(self) -> None:
        if self._session:
            self._session.receipt = ""

    def pending_candidates(self) -> list[PendingCandidate]:
        if not self._session:
            return []
        return list(self._session.pending.values())

    async def mark_candidate_sent(self, capture_id: str) -> None:
        session = self._session
        if not session:
            return
        pending = session.pending.pop(capture_id, None)
        if pending:
            await self._picker.remove_capture(pending.capture)

    async def discard_candidate(self, capture_id: str) -> None:
        await self.mark_candidate_sent(capture_id)

    def health_state(self) -> dict[str, object]:
        if not self._session:
            return {"active": False}
        state = self._session.state
        return {
            "active": True,
            "sessionId": state.session_id,
            "mode": state.mode.value,
            "pageTitle": state.page_title,
        }

    async def _handle_pick(
        self,
        session_id: str,
        source: dict[str, Any],
        handle: Any,
        raw_payload: dict[str, Any],
    ) -> None:
        session = self._session
        if not session or session.state.session_id != session_id:
            return
        try:
            capture = await self._picker.build_capture(source, handle, raw_payload, session.current_url)
        except PickerSecurityError:
            return
        current = self._session
        if not current or current.state.session_id != session_id:
            await self._picker.remove_capture(capture)
            return
        current.pending[capture.capture_id] = PendingCandidate(capture)

    def _require_session(self, session_id: str) -> _ActiveSession:
        if not self._session or self._session.state.session_id != session_id:
            raise CaptureSessionConflict("采集会话不存在或与当前本地会话不一致")
        return self._session

    async def _cleanup_session(self, session: _ActiveSession) -> None:
        for pending in tuple(session.pending.values()):
            await self._picker.remove_capture(pending.capture)
        session.pending.clear()
        session.token = ""
        session.receipt = ""
        await self._cleanup_resources(session.page, session.context, session.lease)
        await self._picker.close()

    async def _cleanup_resources(self, page: Any, context: Any, lease: Any) -> None:
        if page is not None:
            try:
                await self._picker.dispose(page)
            except Exception:
                pass
            try:
                await page.close()
            except Exception:
                pass
        if context is not None:
            try:
                await context.close()
            except Exception:
                pass
        if lease is not None:
            try:
                await lease.close()
            except Exception:
                pass

    def _notify_state(self, active: bool, title: str) -> None:
        if not self._state_callback:
            return
        try:
            self._state_callback(active, title)
        except Exception:
            logger.exception("采集 GUI 状态回调失败")


def _validate_navigation_url(value: str) -> None:
    if not value or any(ord(char) < 32 or ord(char) == 127 for char in value):
        raise ValueError("采集 URL 无效")
    try:
        parsed = urlsplit(value)
        parsed.port
    except ValueError as error:
        raise ValueError("采集 URL 无效") from error
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password:
        raise ValueError("采集 URL 必须是无凭据的 HTTP(S) 地址")


def _sanitize_current_url(value: str) -> str:
    _validate_navigation_url(value)
    parsed = urlsplit(value)
    host = parsed.hostname or ""
    netloc = f"[{host}]" if ":" in host and not host.startswith("[") else host
    if parsed.port:
        netloc = f"{netloc}:{parsed.port}"
    query = [
        (key, item)
        for key, item in parse_qsl(parsed.query, keep_blank_values=True)
        if _normalized_key(key) not in _SENSITIVE_QUERY_KEYS
    ]
    return urlunsplit((parsed.scheme, netloc, parsed.path or "/", urlencode(query, doseq=True), ""))


def _safe_page_title(value: Any) -> str:
    title = " ".join(str(value or "").split())[:120]
    if not title or scan_for_sensitive_data({"pageTitle": title}):
        return "页面元素采集"
    return title


def _normalized_key(value: str) -> str:
    return re.sub(r"[-_.:/\\\s]+", "_", value.strip().lower())
