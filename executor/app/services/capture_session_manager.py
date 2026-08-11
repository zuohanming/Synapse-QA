import asyncio
import contextlib
import logging
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable
from urllib.parse import urlsplit
from uuid import uuid4

from app.core.config import settings
from app.models.capture import CaptureMode, CaptureStartCommand, CaptureState
from app.services.capture_security import (
    CaptureURLSecurityError,
    contains_sensitive_data,
    is_same_capture_site,
    sanitize_public_url,
    validate_network_target,
)
from app.services.element_picker import ElementPicker, LocalCapture, PickerSecurityError


logger = logging.getLogger(__name__)

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
        payload = self.capture.candidate.platform_payload()
        payload["clientCaptureId"] = self.capture.capture_id
        return payload


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
        session_closed_callback: Callable[[CaptureHeartbeatContext, str], Awaitable[None]] | None = None,
        network_validator: Callable[..., str] = validate_network_target,
    ) -> None:
        self._browser_factory = browser_factory or PlaywrightBrowserFactory()
        self._picker = picker or ElementPicker(rate_limit_per_second=settings.capture_rate_limit_per_second)
        self._state_callback = state_callback
        self._session_closed_callback = session_closed_callback
        self._network_validator = network_validator
        self._session: _ActiveSession | None = None
        self._lock = asyncio.Lock()

    def set_state_callback(self, callback: Callable[[bool, str], None] | None) -> None:
        self._state_callback = callback

    def set_session_closed_callback(
        self,
        callback: Callable[[CaptureHeartbeatContext, str], Awaitable[None]] | None,
    ) -> None:
        self._session_closed_callback = callback

    async def start(self, command: CaptureStartCommand) -> CaptureState:
        if command.headless:
            raise ValueError("页面元素采集仅支持有头浏览器")
        if command.browser_channel not in settings.capture_allowed_browser_channels:
            raise ValueError("浏览器通道仅允许 chrome 或 msedge")
        await self._validate_navigation_url(command.url)
        async with self._lock:
            if self._session:
                if self._session.state.session_id == command.session_id:
                    if self._session.token != command.token or self._session.receipt != command.receipt:
                        raise CaptureSessionConflict("重复 start 的令牌或回执不一致")
                    return self._session.state
                raise CaptureSessionConflict("已有页面元素采集会话正在运行")

            lease = browser = context = page = None
            try:
                lease = await self._browser_factory.launch(channel=command.browser_channel, headless=False)
                browser = lease.browser
                context = await browser.new_context()
                page = await context.new_page()
                await context.route("**/*", self._request_guard(command.url, page))
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
                    current_url=sanitize_public_url(command.url),
                    lease=lease,
                    browser=browser,
                    context=context,
                    page=page,
                )
                browser.on("disconnected", lambda: self._schedule_browser_disconnect(command.session_id, browser))

                async def receive_pick(source: dict[str, Any], handle: Any, payload: dict[str, Any]) -> None:
                    await self._handle_pick(command.session_id, source, handle, payload)

                await page.goto(command.url, wait_until="domcontentloaded")
                await self._picker.install(context, page, receive_pick, command.url)
                title = _safe_page_title(await page.title())
                self._session.current_url = sanitize_public_url(page.url)
                self._session.state.page_title = title
                self._notify_state(True, title)
                return self._session.state
            except Exception as error:
                failed = self._session
                self._session = None
                if failed:
                    failed.token = ""
                    failed.receipt = ""
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

    def _schedule_browser_disconnect(self, session_id: str, browser: Any) -> None:
        try:
            asyncio.get_running_loop().create_task(self._handle_browser_disconnect(session_id, browser))
        except RuntimeError:
            logger.warning("浏览器关闭事件未能绑定到采集事件循环")

    async def _handle_browser_disconnect(self, session_id: str, browser: Any) -> None:
        async with self._lock:
            session = self._session
            if not session or session.state.session_id != session_id or session.browser is not browser:
                return
            context = CaptureHeartbeatContext(
                session_id=session.state.session_id,
                token=session.token,
                browser_context_id=session.state.browser_context_id,
                current_url=session.current_url,
                command_receipt=session.receipt,
            )
            self._session = None
            await self._cleanup_session(session)
            self._notify_state(False, "")

        if self._session_closed_callback:
            try:
                await self._session_closed_callback(context, "browser_closed")
            except Exception:
                logger.exception("浏览器关闭后同步采集会话失败")

    def heartbeat_context(self) -> CaptureHeartbeatContext | None:
        session = self._session
        if not session:
            return None
        try:
            session.current_url = sanitize_public_url(session.page.url)
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
            "mode": state.mode.value,
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
        if len(session.pending) >= settings.capture_pending_max:
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
        session.token = ""
        session.receipt = ""
        try:
            for pending in tuple(session.pending.values()):
                with contextlib.suppress(Exception):
                    await self._picker.remove_capture(pending.capture)
            session.pending.clear()
        finally:
            try:
                await self._cleanup_resources(session.page, session.context, session.lease)
            finally:
                with contextlib.suppress(Exception):
                    await self._picker.close()

    async def _cleanup_resources(self, page: Any, context: Any, lease: Any) -> None:
        try:
            if page is not None:
                with contextlib.suppress(Exception):
                    await self._picker.dispose(page)
        finally:
            try:
                if page is not None:
                    with contextlib.suppress(Exception):
                        await page.close()
            finally:
                try:
                    if context is not None:
                        with contextlib.suppress(Exception):
                            await context.close()
                finally:
                    if lease is not None:
                        with contextlib.suppress(Exception):
                            await lease.close()

    async def _validate_navigation_url(self, value: str) -> None:
        try:
            parsed = urlsplit(value)
            if parsed.username or parsed.password:
                raise CaptureURLSecurityError("采集 URL 不允许携带 userinfo")
            sanitize_public_url(value)
            await asyncio.to_thread(
                self._network_validator,
                value,
                allowed_origins=settings.capture_allowed_origins,
                allowed_private_hosts=(urlsplit(value).hostname or "",),
            )
        except CaptureURLSecurityError as error:
            raise ValueError(str(error)) from error

    def _request_guard(self, navigation_url: str, page: Any):
        async def guard(route: Any, request: Any) -> None:
            try:
                request_url = str(request.url)
                allowed_origins = settings.capture_allowed_origins
                navigation_check = getattr(request, "is_navigation_request", False)
                is_navigation_request = navigation_check() if callable(navigation_check) else bool(navigation_check)
                is_main_navigation = bool(is_navigation_request and request.frame is getattr(page, "main_frame", None))
                if is_main_navigation and not is_same_capture_site(request_url, navigation_url):
                    raise CaptureURLSecurityError("顶层导航重定向到不同站点")
                if is_same_capture_site(request_url, navigation_url):
                    # 页面地址在启动采集前已完成 URL 与网络校验；同站点资源不再被二次
                    # DNS 校验阻断，避免 Chrome 将页面请求显示为 ERR_BLOCKED_BY_CLIENT。
                    await route.continue_()
                    return
                await asyncio.to_thread(
                    self._network_validator,
                    request_url,
                    allowed_origins=allowed_origins,
                    allowed_private_hosts=(),
                )
            except CaptureURLSecurityError:
                await route.abort("blockedbyclient")
                return
            await route.continue_()

        return guard

    def _notify_state(self, active: bool, title: str) -> None:
        if not self._state_callback:
            return
        try:
            self._state_callback(active, title)
        except Exception:
            logger.exception("采集 GUI 状态回调失败")


def _safe_page_title(value: Any) -> str:
    title = " ".join(str(value or "").split())[:120]
    if not title or contains_sensitive_data({"pageTitle": title}):
        return "页面元素采集"
    return title
