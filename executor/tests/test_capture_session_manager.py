import asyncio
import os
import threading
from http.server import BaseHTTPRequestHandler, SimpleHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import AsyncMock, Mock

import pytest

from app.models.capture import CaptureCandidate, CaptureCommand, CaptureLocator, CaptureMode, CaptureStartCommand
from app.services.capture_platform_client import (
    CaptureCommandPoller,
    CapturePlatformClient,
    PlatformResponse,
)
from app.services.capture_session_manager import (
    CaptureSessionConflict,
    CaptureSessionManager,
    CaptureStartupError,
    PendingCandidate,
)
from app.services.element_picker import LocalCapture
from gui import ExecutorGui


class FakePage:
    def __init__(self, *, goto_error: Exception | None = None) -> None:
        self.url = "about:blank"
        self.closed = False
        self.goto_error = goto_error
        self.evaluations: list[tuple[object, object]] = []

    async def goto(self, url: str, wait_until: str = "domcontentloaded") -> None:
        assert wait_until == "domcontentloaded"
        if self.goto_error:
            raise self.goto_error
        self.url = url

    async def title(self) -> str:
        return "订单详情"

    async def evaluate(self, expression, argument=None):
        self.evaluations.append((expression, argument))

    async def close(self) -> None:
        self.closed = True


class FakeContext:
    def __init__(self, page: FakePage) -> None:
        self.page = page
        self.closed = False
        self.binding = None
        self.init_scripts: list[str] = []
        self.routes = []

    async def new_page(self) -> FakePage:
        return self.page

    async def expose_binding(self, name, callback, handle=False) -> None:
        self.binding = (name, callback, handle)

    async def add_init_script(self, script: str) -> None:
        self.init_scripts.append(script)

    async def route(self, pattern, handler) -> None:
        self.routes.append((pattern, handler))

    async def close(self) -> None:
        self.closed = True


class FakeBrowser:
    def __init__(self, context: FakeContext) -> None:
        self.context = context
        self.closed = False
        self.disconnected_handlers = []

    def on(self, event: str, handler) -> None:
        assert event == "disconnected"
        self.disconnected_handlers.append(handler)

    def disconnect(self) -> None:
        for handler in tuple(self.disconnected_handlers):
            handler()

    async def new_context(self) -> FakeContext:
        return self.context

    async def close(self) -> None:
        self.closed = True
        self.disconnect()


class FakeBrowserLease:
    def __init__(self, page: FakePage | None = None) -> None:
        self.page = page or FakePage()
        self.context = FakeContext(self.page)
        self.browser = FakeBrowser(self.context)
        self.closed = False

    async def close(self) -> None:
        self.closed = True
        await self.browser.close()


class FakeBrowserFactory:
    def __init__(self, *, page: FakePage | None = None, launch_error: Exception | None = None) -> None:
        self.lease = FakeBrowserLease(page)
        self.launch_error = launch_error
        self.calls: list[tuple[str, bool]] = []

    async def launch(self, *, channel: str, headless: bool) -> FakeBrowserLease:
        self.calls.append((channel, headless))
        if self.launch_error:
            raise self.launch_error
        return self.lease


class FakePicker:
    async def install(self, context, page, callback, capture_url=None) -> None:
        self.install_args = (context, page, callback, capture_url)

    async def set_mode(self, page, mode) -> None:
        self.mode = mode

    async def dispose(self, page) -> None:
        self.disposed = page

    async def close(self) -> None:
        self.closed = True

    async def remove_capture(self, capture) -> None:
        self.removed = capture


def capture_manager(browser_factory, **kwargs) -> CaptureSessionManager:
    return CaptureSessionManager(
        browser_factory=browser_factory,
        picker=kwargs.pop("picker", FakePicker()),
        network_validator=lambda value, **_options: value,
        **kwargs,
    )


def start_command(
    session_id: str = "session-1",
    *,
    command_id: int = 7,
    receipt: str = "start-receipt",
    channel: str = "chrome",
    headless: bool = False,
) -> CaptureStartCommand:
    return CaptureStartCommand(
        id=command_id,
        sessionId=session_id,
        type="start",
        token="session-token",
        receipt=receipt,
        url="https://example.test/orders?tab=details",
        browserChannel=channel,
        mode="pick",
        headless=headless,
    )


@pytest.mark.asyncio
async def test_single_capture_session_is_idempotent_for_same_start_and_rejects_another_session():
    factory = FakeBrowserFactory()
    manager = capture_manager(factory)

    first = await manager.start(start_command())
    duplicate = await manager.start(start_command())

    assert duplicate is first
    assert factory.calls == [("chrome", False)]
    assert manager.heartbeat_context().command_receipt == "start-receipt"
    assert manager.heartbeat_context().token == "session-token"
    with pytest.raises(CaptureSessionConflict, match="已有页面元素采集会话"):
        await manager.start(start_command("session-2"))

    await manager.close()


@pytest.mark.asyncio
@pytest.mark.parametrize("channel", ["chrome", "msedge"])
async def test_capture_only_launches_allowed_headed_channels(channel):
    factory = FakeBrowserFactory()
    manager = capture_manager(factory)

    await manager.start(start_command(channel=channel))

    assert factory.calls == [(channel, False)]
    await manager.close()


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("command", "message"),
    [
        (start_command(channel="chromium"), "浏览器通道"),
        (start_command(headless=True), "仅支持有头"),
    ],
)
async def test_capture_rejects_arbitrary_channel_and_headless_before_launch(command, message):
    factory = FakeBrowserFactory()
    manager = capture_manager(factory)

    with pytest.raises(ValueError, match=message):
        await manager.start(command)

    assert factory.calls == []
    assert not manager.health_state()["active"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "factory",
    [
        FakeBrowserFactory(launch_error=RuntimeError("browser missing")),
        FakeBrowserFactory(page=FakePage(goto_error=RuntimeError("navigation failed"))),
    ],
)
async def test_startup_failure_rolls_back_all_initialized_resources(factory):
    manager = capture_manager(factory)

    with pytest.raises(CaptureStartupError, match="浏览器采集启动失败"):
        await manager.start(start_command())

    assert not manager.health_state()["active"]
    assert manager.heartbeat_context() is None
    if factory.calls and not factory.launch_error:
        assert factory.lease.page.closed
        assert factory.lease.context.closed
        assert factory.lease.closed


@pytest.mark.asyncio
async def test_cleanup_continues_when_each_resource_close_raises():
    calls = []

    class FailingPicker(FakePicker):
        async def dispose(self, page) -> None:
            calls.append("dispose")
            raise RuntimeError("dispose failed")

    class FailingResource:
        def __init__(self, name):
            self.name = name

        async def close(self):
            calls.append(self.name)
            raise RuntimeError(f"{self.name} failed")

    manager = capture_manager(FakeBrowserFactory(), picker=FailingPicker())

    await manager._cleanup_resources(
        FailingResource("page"),
        FailingResource("context"),
        FailingResource("lease"),
    )

    assert calls == ["dispose", "page", "context", "lease"]


@pytest.mark.asyncio
@pytest.mark.parametrize("reason", ["stop", "expire", "unauthorized", "conflict", "shutdown"])
async def test_every_terminal_reason_closes_page_context_browser_and_clears_credentials(reason):
    factory = FakeBrowserFactory()
    manager = capture_manager(factory)
    await manager.start(start_command())

    await manager.stop("session-1", reason)

    assert factory.lease.page.closed
    assert factory.lease.context.closed
    assert factory.lease.closed
    assert manager.heartbeat_context() is None
    assert manager.health_state() == {"active": False}


@pytest.mark.asyncio
async def test_capture_session_does_not_change_normal_task_capacity():
    from app.services.task_manager import TaskManager

    task_manager = TaskManager()
    before = task_manager.stats()
    manager = capture_manager(FakeBrowserFactory())

    await manager.start(start_command())

    assert task_manager.stats() == before == {"queuedTasks": 0, "runningTasks": 0}
    await manager.close()
    task_manager._executor.shutdown(wait=False)


class FakePlatformClient:
    def __init__(self) -> None:
        self.commands: list[CaptureCommand] = []
        self.heartbeat_responses: list[PlatformResponse] = []
        self.heartbeat_receipts: list[str] = []
        self.acks: list[tuple[int, str]] = []
        self.failures: list[str] = []
        self.fail_response = PlatformResponse(200, {})

    async def claim_commands(self) -> PlatformResponse:
        commands, self.commands = self.commands, []
        return PlatformResponse(200, [item.model_dump(by_alias=True, mode="json", exclude_none=True) for item in commands])

    async def heartbeat(self, context) -> PlatformResponse:
        self.heartbeat_receipts.append(context.command_receipt)
        if self.heartbeat_responses:
            return self.heartbeat_responses.pop(0)
        return PlatformResponse(200, {})

    async def ack(self, command_id: int, receipt: str) -> PlatformResponse:
        self.acks.append((command_id, receipt))
        return PlatformResponse(200, {})

    async def fail(self, context, reason: str) -> PlatformResponse:
        self.failures.append(reason)
        return self.fail_response

    async def add_candidate(self, context, payload: dict) -> PlatformResponse:
        return PlatformResponse(201, {})


@pytest.mark.asyncio
async def test_start_receipt_is_sent_once_then_followup_heartbeat_omits_it():
    manager = capture_manager(FakeBrowserFactory())
    client = FakePlatformClient()
    client.commands = [start_command()]
    poller = CaptureCommandPoller(client, manager)

    await poller.run_once()
    await poller.run_once()

    assert client.heartbeat_receipts == ["start-receipt", ""]
    assert client.acks == []
    await manager.close()


@pytest.mark.asyncio
@pytest.mark.parametrize("terminal_type", ["stop", "expire"])
async def test_mode_stop_and_expire_ack_only_after_local_operation_succeeds(terminal_type):
    manager = capture_manager(FakeBrowserFactory())
    client = FakePlatformClient()
    poller = CaptureCommandPoller(client, manager)
    client.commands = [start_command()]
    await poller.run_once()

    client.commands = [
        CaptureCommand(id=8, sessionId="session-1", type="set_mode", mode="operate", receipt="mode-receipt"),
    ]
    await poller.run_once()
    assert manager.health_state()["mode"] == "operate"
    assert client.acks == [(8, "mode-receipt")]

    terminal_receipt = f"{terminal_type}-receipt"
    client.commands = [
        CaptureCommand(id=9, sessionId="session-1", type=terminal_type, receipt=terminal_receipt)
    ]
    await poller.run_once()
    assert client.acks[-1] == (9, terminal_receipt)
    assert not manager.health_state()["active"]


@pytest.mark.asyncio
async def test_network_error_keeps_session_and_start_receipt_for_next_cycle():
    manager = capture_manager(FakeBrowserFactory())
    client = FakePlatformClient()
    client.commands = [start_command()]
    client.heartbeat_responses = [PlatformResponse(0, None), PlatformResponse(200, {})]
    poller = CaptureCommandPoller(client, manager)

    await poller.run_once()
    assert manager.health_state()["active"]
    assert manager.heartbeat_context().command_receipt == "start-receipt"
    await poller.run_once()

    assert client.heartbeat_receipts == ["start-receipt", "start-receipt"]
    assert manager.heartbeat_context().command_receipt == ""
    await manager.close()


@pytest.mark.asyncio
async def test_manual_browser_close_stops_local_capture_and_reports_platform_failure():
    factory = FakeBrowserFactory()
    manager = capture_manager(factory)
    client = FakePlatformClient()
    client.commands = [start_command()]
    poller = CaptureCommandPoller(client, manager)

    await poller.run_once()
    factory.lease.browser.disconnect()
    await asyncio.sleep(0)
    await asyncio.sleep(0)

    assert manager.health_state() == {"active": False}
    assert client.failures == ["browser_closed"]


@pytest.mark.asyncio
async def test_duplicate_start_never_rotates_credentials_without_relaunching_browser():
    factory = FakeBrowserFactory()
    manager = capture_manager(factory)
    await manager.start(start_command(receipt="lease-1"))
    redelivery = start_command(command_id=8, receipt="lease-2").model_copy(update={"token": "rotated-token"})

    with pytest.raises(CaptureSessionConflict, match="令牌或回执不一致"):
        await manager.start(redelivery)

    assert factory.calls == [("chrome", False)]
    assert manager.heartbeat_context().command_receipt == "lease-1"
    assert manager.heartbeat_context().token == "session-token"
    await manager.close()


@pytest.mark.asyncio
@pytest.mark.parametrize("status", [401, 409])
async def test_unauthorized_and_conflict_heartbeat_close_local_session(status):
    auth_failure = Mock()
    manager = capture_manager(FakeBrowserFactory())
    client = FakePlatformClient()
    client.commands = [start_command()]
    client.heartbeat_responses = [PlatformResponse(status, {})]
    poller = CaptureCommandPoller(client, manager, on_auth_failure=auth_failure)

    await poller.run_once()

    assert not manager.health_state()["active"]
    assert auth_failure.call_count == (1 if status == 401 else 0)


@pytest.mark.asyncio
async def test_any_401_terminates_active_session_even_when_response_session_id_differs():
    manager = capture_manager(FakeBrowserFactory())
    await manager.start(start_command())
    poller = CaptureCommandPoller(FakePlatformClient(), manager)

    await poller._handle_terminal_response(PlatformResponse(401, None), "stale-session")

    assert manager.heartbeat_context() is None


@pytest.mark.asyncio
async def test_start_failure_callback_401_notifies_gui_auth_failure_without_leaking_resources():
    auth_failure = Mock()
    manager = capture_manager(FakeBrowserFactory(launch_error=RuntimeError("missing")))
    client = FakePlatformClient()
    client.commands = [start_command()]
    client.fail_response = PlatformResponse(401, {})
    poller = CaptureCommandPoller(client, manager, on_auth_failure=auth_failure)

    await poller.run_once()

    assert client.failures == ["browser_start_failed"]
    assert not manager.health_state()["active"]
    auth_failure.assert_called_once()


@pytest.mark.asyncio
async def test_command_poller_waits_exactly_two_seconds_and_closes_manager_on_shutdown():
    manager = Mock()
    manager.close = Mock(side_effect=lambda: asyncio.sleep(0))
    manager.heartbeat_context.return_value = None
    manager.pending_candidates.return_value = []
    sleep_calls: list[float] = []
    poller = CaptureCommandPoller(FakePlatformClient(), manager)

    async def stop_after_first_wait(seconds: float) -> None:
        sleep_calls.append(seconds)
        poller.request_stop()

    poller._sleep = stop_after_first_wait
    await poller.run_forever()

    assert sleep_calls == [2]
    manager.close.assert_called_once()


@pytest.mark.asyncio
async def test_platform_client_uses_long_token_for_claim_and_session_bearer_for_callbacks():
    requests = []

    async def transport(method, url, headers, payload):
        requests.append((method, url, headers, payload))
        return PlatformResponse(200, {"data": []})

    client = CapturePlatformClient(
        base_url="https://platform.test",
        executor_id="exec-1",
        executor_token="long-token",
        transport=transport,
    )
    await client.claim_commands()
    context = Mock(
        session_id="session-1",
        token="session-token",
        browser_context_id="ctx-1",
        current_url="https://example.test/orders",
        command_receipt="receipt",
    )
    await client.heartbeat(context)

    claim = requests[0]
    callback = requests[1]
    assert claim[0] == "GET"
    assert "executorId=exec-1" in claim[1]
    assert claim[2]["X-Executor-Token"] == "long-token"
    assert callback[2]["Authorization"] == "Bearer session-token"
    assert callback[2]["X-Executor-ID"] == "exec-1"
    assert callback[3]["commandReceipt"] == "receipt"
    assert "token" not in callback[3]


@pytest.mark.asyncio
async def test_platform_client_never_follows_cross_origin_redirect_with_credentials():
    stolen_headers = []

    class SinkHandler(BaseHTTPRequestHandler):
        def do_GET(self):
            stolen_headers.append(dict(self.headers))
            self.send_response(200)
            self.end_headers()

        do_POST = do_GET

        def log_message(self, *_args):
            return

    sink = ThreadingHTTPServer(("127.0.0.1", 0), SinkHandler)
    sink_thread = threading.Thread(target=sink.serve_forever, daemon=True)
    sink_thread.start()

    class RedirectHandler(BaseHTTPRequestHandler):
        def _redirect(self):
            self.send_response(302)
            self.send_header("Location", f"http://127.0.0.1:{sink.server_address[1]}/steal")
            self.end_headers()

        do_GET = _redirect
        do_POST = _redirect

        def log_message(self, *_args):
            return

    source = ThreadingHTTPServer(("127.0.0.1", 0), RedirectHandler)
    source_thread = threading.Thread(target=source.serve_forever, daemon=True)
    source_thread.start()
    try:
        client = CapturePlatformClient(
            base_url=f"http://127.0.0.1:{source.server_address[1]}",
            executor_id="exec-1",
            executor_token="long-token",
        )
        context = Mock(
            session_id="session-1",
            token="session-token",
            browser_context_id="ctx-1",
            current_url="https://example.test/orders",
            command_receipt="receipt",
        )

        claim = await client.claim_commands()
        heartbeat = await client.heartbeat(context)

        assert claim.status == 302
        assert heartbeat.status == 302
        assert stolen_headers == []
    finally:
        source.shutdown()
        source.server_close()
        source_thread.join(timeout=2)
        sink.shutdown()
        sink.server_close()
        sink_thread.join(timeout=2)


@pytest.mark.asyncio
async def test_platform_client_rejects_oversized_response_body():
    class OversizedHandler(BaseHTTPRequestHandler):
        def do_GET(self):
            payload = b"x" * (1024 * 1024 + 2)
            self.send_response(200)
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def log_message(self, *_args):
            return

    server = ThreadingHTTPServer(("127.0.0.1", 0), OversizedHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        client = CapturePlatformClient(
            base_url=f"http://127.0.0.1:{server.server_address[1]}",
            executor_id="exec-1",
            executor_token="long-token",
        )

        response = await client.claim_commands()

        assert response == PlatformResponse(0, None)
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def test_pending_candidate_retry_keeps_stable_client_capture_id():
    candidate = CaptureCandidate(
        name="保存",
        fingerprint="a" * 64,
        captureUrl="https://example.test/orders",
        tagName="button",
        accessibleName="保存",
        locators=[CaptureLocator(type="id", value="save", score=90, unique=True, matchCount=1)],
        qualityScore=90,
    )
    pending = PendingCandidate(LocalCapture("550e8400-e29b-41d4-a716-446655440000", candidate, None))

    assert pending.payload["clientCaptureId"] == "550e8400-e29b-41d4-a716-446655440000"
    assert pending.payload == pending.payload


def test_gui_capture_status_is_dispatched_to_tk_main_thread():
    gui = ExecutorGui.__new__(ExecutorGui)
    gui.root = Mock()
    gui._apply_capture_status = Mock()
    gui.ui_event_queue = __import__("queue").Queue()
    gui.closing = True

    worker = threading.Thread(target=gui.queue_capture_status, args=(True, "订单详情"))
    worker.start()
    worker.join(timeout=1)

    gui.root.after.assert_not_called()
    gui._drain_ui_events()
    gui._apply_capture_status.assert_called_once_with(True, "订单详情")


def test_fastapi_lifecycle_starts_one_capture_poller_and_health_is_non_sensitive(monkeypatch):
    from fastapi.testclient import TestClient
    from app.api import routes
    from app.core.config import settings

    heartbeat_start = Mock()
    heartbeat_stop = Mock()
    poller_start = Mock()
    poller_stop = Mock()
    manager_close = AsyncMock()
    monkeypatch.setattr(routes.heartbeat_client, "start", heartbeat_start)
    monkeypatch.setattr(routes.heartbeat_client, "stop", heartbeat_stop)
    monkeypatch.setattr(routes.capture_command_poller, "start", poller_start)
    monkeypatch.setattr(routes.capture_command_poller, "stop", poller_stop)
    monkeypatch.setattr(routes.capture_session_manager, "close", manager_close)
    monkeypatch.setattr(
        routes.capture_session_manager,
        "health_state",
        lambda: {"active": True, "mode": "pick"},
    )

    with TestClient(routes.create_app()) as client:
        response = client.get("/health")

    assert response.status_code == 200
    assert response.json()["capture"] == {"active": True, "mode": "pick"}
    assert "token" not in str(response.json()).lower()
    assert "receipt" not in str(response.json()).lower()
    assert "url" not in str(response.json()).lower()
    heartbeat_start.assert_called_once()
    heartbeat_stop.assert_called_once()
    poller_start.assert_called_once()
    poller_stop.assert_called_once()
    manager_close.assert_awaited_once()
    assert settings.capture_command_poll_interval_seconds == 2


def test_gui_registration_connects_thread_safe_capture_status_and_shared_auth_failure(monkeypatch):
    from app.api import routes

    gui = ExecutorGui.__new__(ExecutorGui)
    gui.root = Mock()
    capture_handler = Mock()
    auth_handler = Mock()
    poller_stop = Mock()
    monkeypatch.setattr(routes.capture_session_manager, "set_state_callback", capture_handler)
    monkeypatch.setattr(routes.heartbeat_client, "set_auth_failure_handler", auth_handler)
    monkeypatch.setattr(routes.capture_command_poller, "request_stop", poller_stop)
    gui_auth = Mock()

    routes.configure_gui_callbacks(
        gui.queue_capture_status,
        gui_auth,
    )

    capture_handler.assert_called_once_with(gui.queue_capture_status)
    auth_handler.assert_called_once()
    registered_auth = auth_handler.call_args.args[0]
    registered_auth()
    poller_stop.assert_called_once()
    gui_auth.assert_called_once()


@pytest.mark.asyncio
async def test_headed_manager_uses_explicit_local_allowlist_and_blocks_metadata(tmp_path, monkeypatch):
    if os.getenv("RUN_HEADED_PICKER_INTEGRATION") != "1":
        pytest.skip("需设置 RUN_HEADED_PICKER_INTEGRATION=1 才会打开本机有头 Edge")
    pytest.importorskip("playwright.async_api", reason="未安装 Playwright Python 包")
    from app.core.config import settings

    (tmp_path / "index.html").write_text(
        "<!doctype html><meta charset='utf-8'><title>Manager Integration</title><button id='save'>保存</button>",
        encoding="utf-8",
    )

    class QuietHandler(SimpleHTTPRequestHandler):
        def log_message(self, *_args):
            return

    handler = lambda *args, **kwargs: QuietHandler(*args, directory=str(tmp_path), **kwargs)
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    origin = f"http://127.0.0.1:{server.server_address[1]}"
    monkeypatch.setattr(settings, "capture_allowed_origins", (origin,))
    monkeypatch.setattr(settings, "capture_allowed_private_hosts", ("127.0.0.1",))
    manager = CaptureSessionManager()
    try:
        command = start_command().model_copy(
            update={"url": f"{origin}/index.html?temporary=secret#fragment", "browser_channel": "msedge"}
        )
        await manager.start(command)
        context = manager.heartbeat_context()

        assert context.current_url == f"{origin}/index.html"
        assert manager.health_state() == {"active": True, "mode": "pick"}
        result = await manager._session.page.evaluate(
            """async () => {
              try { await fetch("http://169.254.169.254/latest/meta-data/"); return "unexpected"; }
              catch (_) { return "blocked"; }
            }"""
        )
        assert result == "blocked"
    finally:
        await manager.close()
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)
    assert manager.health_state() == {"active": False}
