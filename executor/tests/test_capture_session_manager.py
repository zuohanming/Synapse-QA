import asyncio
import threading
from unittest.mock import Mock

import pytest

from app.models.capture import CaptureCommand, CaptureMode, CaptureStartCommand
from app.services.capture_platform_client import (
    CaptureCommandPoller,
    CapturePlatformClient,
    PlatformResponse,
)
from app.services.capture_session_manager import (
    CaptureSessionConflict,
    CaptureSessionManager,
    CaptureStartupError,
)
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

    async def new_page(self) -> FakePage:
        return self.page

    async def expose_binding(self, name, callback, handle=False) -> None:
        self.binding = (name, callback, handle)

    async def add_init_script(self, script: str) -> None:
        self.init_scripts.append(script)

    async def close(self) -> None:
        self.closed = True


class FakeBrowser:
    def __init__(self, context: FakeContext) -> None:
        self.context = context
        self.closed = False

    async def new_context(self) -> FakeContext:
        return self.context

    async def close(self) -> None:
        self.closed = True


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
    manager = CaptureSessionManager(browser_factory=factory)

    first = await manager.start(start_command())
    duplicate = await manager.start(start_command(command_id=8, receipt="new-receipt"))

    assert duplicate is first
    assert factory.calls == [("chrome", False)]
    assert manager.heartbeat_context().command_receipt == "new-receipt"
    assert manager.heartbeat_context().token == "session-token"
    with pytest.raises(CaptureSessionConflict, match="已有页面元素采集会话"):
        await manager.start(start_command("session-2"))

    await manager.close()


@pytest.mark.asyncio
@pytest.mark.parametrize("channel", ["chrome", "msedge"])
async def test_capture_only_launches_allowed_headed_channels(channel):
    factory = FakeBrowserFactory()
    manager = CaptureSessionManager(browser_factory=factory)

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
    manager = CaptureSessionManager(browser_factory=factory)

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
    manager = CaptureSessionManager(browser_factory=factory)

    with pytest.raises(CaptureStartupError, match="浏览器采集启动失败"):
        await manager.start(start_command())

    assert not manager.health_state()["active"]
    assert manager.heartbeat_context() is None
    if factory.calls and not factory.launch_error:
        assert factory.lease.page.closed
        assert factory.lease.context.closed
        assert factory.lease.closed


@pytest.mark.asyncio
@pytest.mark.parametrize("reason", ["stop", "expire", "unauthorized", "conflict", "shutdown"])
async def test_every_terminal_reason_closes_page_context_browser_and_clears_credentials(reason):
    factory = FakeBrowserFactory()
    manager = CaptureSessionManager(browser_factory=factory)
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
    manager = CaptureSessionManager(browser_factory=FakeBrowserFactory())

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
    manager = CaptureSessionManager(browser_factory=FakeBrowserFactory())
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
    manager = CaptureSessionManager(browser_factory=FakeBrowserFactory())
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
    manager = CaptureSessionManager(browser_factory=FakeBrowserFactory())
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
async def test_released_start_redelivery_rotates_credentials_without_relaunching_browser():
    factory = FakeBrowserFactory()
    manager = CaptureSessionManager(browser_factory=factory)
    await manager.start(start_command(receipt="lease-1"))
    redelivery = start_command(command_id=8, receipt="lease-2").model_copy(update={"token": "rotated-token"})

    await manager.start(redelivery)

    assert factory.calls == [("chrome", False)]
    assert manager.heartbeat_context().command_receipt == "lease-2"
    assert manager.heartbeat_context().token == "rotated-token"
    await manager.close()


@pytest.mark.asyncio
@pytest.mark.parametrize("status", [401, 409])
async def test_unauthorized_and_conflict_heartbeat_close_local_session(status):
    auth_failure = Mock()
    manager = CaptureSessionManager(browser_factory=FakeBrowserFactory())
    client = FakePlatformClient()
    client.commands = [start_command()]
    client.heartbeat_responses = [PlatformResponse(status, {})]
    poller = CaptureCommandPoller(client, manager, on_auth_failure=auth_failure)

    await poller.run_once()

    assert not manager.health_state()["active"]
    assert auth_failure.call_count == (1 if status == 401 else 0)


@pytest.mark.asyncio
async def test_start_failure_callback_401_notifies_gui_auth_failure_without_leaking_resources():
    auth_failure = Mock()
    manager = CaptureSessionManager(browser_factory=FakeBrowserFactory(launch_error=RuntimeError("missing")))
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


def test_gui_capture_status_is_dispatched_to_tk_main_thread():
    gui = ExecutorGui.__new__(ExecutorGui)
    gui.root = Mock()
    gui._apply_capture_status = Mock()

    worker = threading.Thread(target=gui.queue_capture_status, args=(True, "订单详情"))
    worker.start()
    worker.join(timeout=1)

    gui.root.after.assert_called_once()
    delay, callback, active, title = gui.root.after.call_args.args
    assert delay == 0
    assert callback == gui._apply_capture_status
    assert (active, title) == (True, "订单详情")


def test_fastapi_lifecycle_starts_one_capture_poller_and_health_is_non_sensitive(monkeypatch):
    from fastapi.testclient import TestClient
    from app.api import routes
    from app.core.config import settings

    heartbeat_start = Mock()
    heartbeat_stop = Mock()
    poller_start = Mock()
    poller_stop = Mock()
    monkeypatch.setattr(routes.heartbeat_client, "start", heartbeat_start)
    monkeypatch.setattr(routes.heartbeat_client, "stop", heartbeat_stop)
    monkeypatch.setattr(routes.capture_command_poller, "start", poller_start)
    monkeypatch.setattr(routes.capture_command_poller, "stop", poller_stop)
    monkeypatch.setattr(
        routes.capture_session_manager,
        "health_state",
        lambda: {"active": True, "mode": "pick", "pageTitle": "订单详情"},
    )

    with TestClient(routes.create_app()) as client:
        response = client.get("/health")

    assert response.status_code == 200
    assert response.json()["capture"] == {"active": True, "mode": "pick", "pageTitle": "订单详情"}
    assert "token" not in str(response.json()).lower()
    assert "receipt" not in str(response.json()).lower()
    assert "url" not in str(response.json()).lower()
    heartbeat_start.assert_called_once()
    heartbeat_stop.assert_called_once()
    poller_start.assert_called_once()
    poller_stop.assert_called_once()
    assert settings.capture_command_poll_interval_seconds == 2


def test_gui_registration_connects_thread_safe_capture_status_and_shared_auth_failure(monkeypatch):
    from app.api import routes

    gui = ExecutorGui.__new__(ExecutorGui)
    gui.root = Mock()
    capture_handler = Mock()
    auth_handler = Mock()
    monkeypatch.setattr(routes.capture_session_manager, "set_state_callback", capture_handler)
    monkeypatch.setattr(routes.heartbeat_client, "set_auth_failure_handler", auth_handler)

    routes.configure_gui_callbacks(
        gui.queue_capture_status,
        lambda: gui.root.after(0, gui._auth_expired),
    )

    capture_handler.assert_called_once_with(gui.queue_capture_status)
    auth_handler.assert_called_once()
    registered_auth = auth_handler.call_args.args[0]
    registered_auth()
    gui.root.after.assert_called_once_with(0, gui._auth_expired)
