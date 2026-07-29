import asyncio
import os
import threading
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import patch

import pytest

from app.models.capture import CaptureCandidate, CaptureLocator, CaptureMode
from app.services.element_picker import (
    ElementPicker,
    PickerSecurityError,
    scan_for_sensitive_data,
)


class FakeLocator:
    def __init__(self, count: int = 1, *, sensitive: bool = False) -> None:
        self._count = count
        self.sensitive = sensitive

    async def count(self) -> int:
        return self._count

    def nth(self, _index: int):
        return self

    async def evaluate(self, _expression):
        return self.sensitive


class FakeFrame:
    def __init__(self, count: int = 1) -> None:
        self.count_value = count
        self.evaluations: list[tuple[object, object]] = []
        self.url = "https://example.test/orders"
        self.parent_frame = None
        self.page = None
        self.controllers = []

    def locator(self, selector: str) -> FakeLocator:
        if selector.startswith("input"):
            return FakeLocator(0)
        return FakeLocator(self.count_value)

    def get_by_test_id(self, _value: str) -> FakeLocator:
        return FakeLocator(self.count_value)

    def get_by_role(self, _role: str, *, name: str) -> FakeLocator:
        return FakeLocator(self.count_value)

    def get_by_label(self, _value: str, *, exact: bool) -> FakeLocator:
        return FakeLocator(self.count_value)

    def get_by_text(self, _value: str, *, exact: bool) -> FakeLocator:
        return FakeLocator(self.count_value)

    async def evaluate(self, expression, argument=None):
        self.evaluations.append((expression, argument))

    async def evaluate_handle(self, expression, argument=None):
        controller = FakeController()
        self.evaluations.append((expression, argument))
        self.controllers.append(controller)
        return controller


class FakeController:
    def __init__(self) -> None:
        self.commands = []
        self.disposed = False

    async def evaluate(self, _expression, command):
        self.commands.append(command)

    async def dispose(self):
        self.disposed = True


class FakeElementHandle:
    def __init__(self, frame=None, payload=None) -> None:
        self.frame = frame
        self.payload = payload or safe_payload()
        self.scrolled = False

    async def scroll_into_view_if_needed(self) -> None:
        self.scrolled = True

    async def bounding_box(self):
        return {"x": 10, "y": 20, "width": 120, "height": 40}

    async def owner_frame(self):
        return self.frame

    async def evaluate(self, _expression):
        return self.payload


class FakePickerContext:
    def __init__(self) -> None:
        self.binding = None
        self.scripts = []

    async def expose_binding(self, name, callback, handle=False) -> None:
        self.binding = (name, callback, handle)

    async def add_init_script(self, script: str) -> None:
        self.scripts.append(script)


class FakePickerPage:
    def __init__(self, frame=None, *, screenshot_error=None) -> None:
        self.evaluations = []
        self.url = "https://example.test/orders"
        self.main_frame = frame or FakeFrame()
        self.frames = [self.main_frame]
        self.main_frame.page = self
        self.events = {}
        self.screenshot_error = screenshot_error
        self.screenshots = []

    def on(self, name, callback):
        self.events[name] = callback

    async def screenshot(self, **options):
        if self.screenshot_error:
            raise self.screenshot_error
        Path(options["path"]).write_bytes(b"png")
        self.screenshots.append(options)


def fake_source(count=1, *, screenshot_error=None):
    frame = FakeFrame(count)
    page = FakePickerPage(frame, screenshot_error=screenshot_error)
    return {"frame": frame, "page": page}, frame, page


class FakeProperty:
    def __init__(self, value=None, element=None) -> None:
        self.value = value
        self.element = element
        self.disposed = False

    async def json_value(self):
        return self.value

    def as_element(self):
        return self.element

    async def dispose(self):
        self.disposed = True


class FakeBundle(FakeProperty):
    def __init__(self, values) -> None:
        super().__init__()
        self.values = values

    async def get_property(self, name):
        value = self.values.get(name)
        if name == "element":
            return FakeProperty(element=value)
        return FakeProperty(value=value)


def safe_payload(**changes):
    payload = {
        "tag": "button",
        "attributes": {"id": "save-order", "role": "button"},
        "accessibleName": "保存订单",
        "label": "",
        "visibleText": "保存订单",
        "role": "button",
        "depth": 3,
        "path": [{"tag": "button", "nth": 1}],
        "locatorMatches": {},
    }
    payload.update(changes)
    return payload


@pytest.mark.asyncio
async def test_picker_installs_structured_binding_and_supports_mode_clear_and_dispose():
    picker = ElementPicker()
    context = FakePickerContext()
    page = FakePickerPage()

    await picker.install(context, page, lambda *_args: None, page.url)
    await picker.set_mode(page, CaptureMode.operate)
    await picker.clear_highlight(page)
    await picker.dispose(page)

    assert context.binding[0] == "__synapseCapturePick"
    assert context.binding[2] is True
    assert context.scripts == []
    assert "__synapseCapturePicker" not in picker.script
    install_options = page.main_frame.evaluations[0][1]
    assert len(install_options["nonce"]) >= 43
    assert install_options["nonce"] not in picker.script
    actions = page.main_frame.controllers[0].commands
    assert actions == [
        {"action": "mode", "mode": "operate"},
        {"action": "clear"},
        {"action": "dispose"},
    ]


@pytest.mark.asyncio
async def test_picker_installs_once_per_same_origin_document_and_never_cross_origin():
    picker = ElementPicker()
    context = FakePickerContext()
    page = FakePickerPage()
    child = FakeFrame()
    child.parent_frame = page.main_frame
    child.page = page
    cross = FakeFrame()
    cross.url = "https://cross.example/frame"
    cross.parent_frame = page.main_frame
    cross.page = page
    page.frames.extend([child, cross])

    await picker.install(context, page, lambda *_args: None, page.url)
    await picker.install_on_page(page)
    await picker.set_mode(page, CaptureMode.operate)

    assert len(page.main_frame.controllers) == 1
    assert len(child.controllers) == 1
    assert cross.controllers == []
    assert page.main_frame.controllers[0].commands == [{"action": "mode", "mode": "operate"}]
    assert child.controllers[0].commands == [{"action": "mode", "mode": "operate"}]


@pytest.mark.asyncio
async def test_binding_requires_nonce_current_page_frame_handle_and_matching_dom():
    picker = ElementPicker(rate_limit_per_second=5)
    context = FakePickerContext()
    page = FakePickerPage()
    received = []

    async def callback(*args):
        received.append(args)

    await picker.install(context, page, callback, page.url)
    nonce = page.main_frame.evaluations[0][1]["nonce"]
    binding = context.binding[1]
    element = FakeElementHandle(page.main_frame, safe_payload())
    source = {"page": page, "frame": page.main_frame}

    await binding(source, FakeBundle({"nonce": "wrong", "action": "pick", "element": element, "payload": safe_payload()}))
    await binding(
        {"page": FakePickerPage(), "frame": page.main_frame},
        FakeBundle({"nonce": nonce, "action": "pick", "element": element, "payload": safe_payload()}),
    )
    await binding(
        source,
        FakeBundle(
            {
                "nonce": nonce,
                "action": "pick",
                "element": element,
                "payload": safe_payload(visibleText="网页伪造"),
            }
        ),
    )
    await binding(source, FakeBundle({"nonce": nonce, "action": "pick", "element": element, "payload": safe_payload()}))

    assert len(received) == 1
    assert received[0][1] is element


@pytest.mark.asyncio
async def test_binding_rate_limit_discards_before_candidate_callback():
    picker = ElementPicker(rate_limit_per_second=2)
    context = FakePickerContext()
    page = FakePickerPage()
    received = []

    async def callback(*args):
        received.append(args)

    await picker.install(context, page, callback, page.url)
    nonce = page.main_frame.evaluations[0][1]["nonce"]
    binding = context.binding[1]
    source = {"page": page, "frame": page.main_frame}
    for _ in range(4):
        element = FakeElementHandle(page.main_frame, safe_payload())
        await binding(source, FakeBundle({"nonce": nonce, "action": "pick", "element": element, "payload": safe_payload()}))

    assert len(received) == 2


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "field",
    ["value", "cookie", "localStorage", "sessionStorage", "outerHTML", "authorization", "selector", "cssSelector", "xpath"],
)
async def test_dom_binding_rejects_every_non_allowlisted_or_sensitive_field(field, tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    payload = safe_payload()
    payload[field] = "must-not-cross-boundary"
    source, frame, _ = fake_source()

    with pytest.raises(PickerSecurityError, match="字段"):
        await picker.build_capture(
            source,
            FakeElementHandle(frame),
            payload,
            "https://example.test/orders",
        )

    assert list(tmp_path.iterdir()) == []


@pytest.mark.asyncio
async def test_dom_binding_keeps_only_fixed_safe_attribute_allowlist(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    payload = safe_payload(
        attributes={
            "id": "save-order",
            "class": "primary",
            "role": "button",
            "data-testid": "save-order",
            "data-unknown": "ignored",
            "onclick": "ignored",
        }
    )
    source, frame, _ = fake_source()

    capture = await picker.build_capture(
        source,
        FakeElementHandle(frame),
        payload,
        "https://example.test/orders",
    )

    serialized = str(capture.candidate.platform_payload())
    assert "save-order" in serialized
    assert "data-unknown" not in serialized
    assert "onclick" not in serialized
    await picker.remove_capture(capture)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "secret",
    [
        "Bearer sk-live-4f052e637b014b2c91e29a58",
        "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature",
        "ghp_abcdefghijklmnopqrstuvwxyz1234567890",
    ],
)
async def test_sensitive_scan_rejects_candidate_before_task5_without_logging_raw_value(secret, tmp_path, caplog):
    picker = ElementPicker(temp_root=tmp_path)
    payload = safe_payload(attributes={"data-testid": secret})
    source, frame, _ = fake_source()

    with pytest.raises(PickerSecurityError, match="敏感"):
        await picker.build_capture(
            source,
            FakeElementHandle(frame),
            payload,
            "https://example.test/orders",
        )

    assert secret not in caplog.text
    assert list(tmp_path.iterdir()) == []


@pytest.mark.asyncio
async def test_second_sensitive_scan_rejects_compromised_task5_output(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    unsafe_candidate = CaptureCandidate(
        name="保存",
        fingerprint="a" * 64,
        captureUrl="https://example.test/orders",
        tagName="button",
        accessibleName="保存",
        locators=[CaptureLocator(type="id", value="safe", score=90, unique=True, matchCount=1)],
        qualityScore=90,
    )
    unsafe_payload = {
        "name": "保存",
        "fingerprint": "a" * 64,
        "captureUrl": "https://example.test/orders",
        "tagName": "button",
        "accessibleName": "保存",
        "locators": [{"type": "id", "value": "Bearer top-secret-token", "score": 90, "unique": True}],
        "qualityScore": 90,
    }
    source, frame, _ = fake_source()

    with patch("app.services.element_picker.build_candidate", return_value=unsafe_candidate), patch.object(
        CaptureCandidate, "platform_payload", return_value=unsafe_payload
    ):
        with pytest.raises(PickerSecurityError, match="敏感"):
            await picker.build_capture(
                source,
                FakeElementHandle(frame),
                safe_payload(),
                "https://example.test/orders",
            )

    assert list(tmp_path.iterdir()) == []


@pytest.mark.asyncio
async def test_match_counts_are_recomputed_through_trusted_frame_locators(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    source, frame, _ = fake_source(count=2)

    capture = await picker.build_capture(
        source,
        FakeElementHandle(frame),
        safe_payload(locatorMatches={"id:save-order": 1}),
        "https://example.test/orders",
    )

    assert all(not locator.unique for locator in capture.candidate.locators)
    assert all(locator.match_count == 2 for locator in capture.candidate.locators)
    await picker.remove_capture(capture)


@pytest.mark.asyncio
async def test_local_screenshot_masks_sensitive_inputs_and_is_deleted_after_report(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    source, frame, page = fake_source()
    handle = FakeElementHandle(frame)

    capture = await picker.build_capture(
        source,
        handle,
        safe_payload(),
        "https://example.test/orders",
    )

    assert capture.screenshot_path is not None
    assert capture.screenshot_path.exists()
    assert handle.scrolled
    assert page.screenshots[0]["clip"] == {"x": 10.0, "y": 20.0, "width": 120.0, "height": 40.0}
    assert page.screenshots[0]["mask_color"] == "#202124"
    assert frame.evaluations == []
    await picker.remove_capture(capture)
    assert not capture.screenshot_path.exists()


@pytest.mark.asyncio
async def test_screenshot_failure_removes_mask_and_partial_temp_file(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    source, frame, _ = fake_source(screenshot_error=RuntimeError("screenshot failed"))

    with pytest.raises(RuntimeError, match="screenshot failed"):
        await picker.build_capture(
            source,
            FakeElementHandle(frame),
            safe_payload(),
            "https://example.test/orders",
        )

    assert frame.evaluations == []
    assert list(tmp_path.rglob("*.png")) == []


@pytest.mark.asyncio
async def test_screenshots_are_serialized(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    source, frame, page = fake_source()
    page.active = 0
    page.max_active = 0

    async def slow_screenshot(**options):
        page.active += 1
        page.max_active = max(page.max_active, page.active)
        await asyncio.sleep(0.02)
        Path(options["path"]).write_bytes(b"png")
        page.active -= 1

    page.screenshot = slow_screenshot
    first, second = await asyncio.gather(
        picker._screenshot(page, FakeElementHandle(frame)),
        picker._screenshot(page, FakeElementHandle(frame)),
    )

    assert page.max_active == 1
    picker._remove_file(first)
    picker._remove_file(second)


@pytest.mark.asyncio
async def test_file_delete_failure_is_retained_for_retry_and_does_not_raise(tmp_path, monkeypatch):
    picker = ElementPicker(temp_root=tmp_path)
    source, frame, _ = fake_source()
    capture = await picker.build_capture(source, FakeElementHandle(frame), safe_payload(), "https://example.test/orders")
    real_unlink = Path.unlink

    def fail_unlink(self, *args, **kwargs):
        if self == capture.screenshot_path:
            raise PermissionError("busy")
        return real_unlink(self, *args, **kwargs)

    monkeypatch.setattr(Path, "unlink", fail_unlink)
    await picker.remove_capture(capture)
    assert capture.screenshot_path in picker._retry_files
    assert capture.screenshot_path.exists()

    monkeypatch.setattr(Path, "unlink", real_unlink)
    await picker.close()
    assert not capture.screenshot_path.exists()


@pytest.mark.asyncio
async def test_delete_cleanup_error_never_overrides_screenshot_error(tmp_path, monkeypatch):
    picker = ElementPicker(temp_root=tmp_path)
    _, frame, page = fake_source(screenshot_error=RuntimeError("main screenshot error"))
    real_unlink = Path.unlink
    monkeypatch.setattr(Path, "unlink", lambda *_args, **_kwargs: (_ for _ in ()).throw(PermissionError("busy")))

    with pytest.raises(RuntimeError, match="main screenshot error"):
        await picker._screenshot(page, FakeElementHandle(frame))

    assert picker._retry_files
    monkeypatch.setattr(Path, "unlink", real_unlink)
    await picker.close()


@pytest.mark.asyncio
@pytest.mark.skipif(os.name != "nt", reason="仅验证 Windows 临时目录安全边界")
async def test_windows_temp_file_security_does_not_claim_posix_chmod_as_dacl(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)

    with patch("app.services.element_picker.os.chmod") as chmod:
        picker._new_screenshot_path()

    chmod.assert_not_called()
    await picker.close()


def test_recursive_sensitive_scanner_handles_keys_and_values_without_false_positive():
    assert scan_for_sensitive_data({"nested": [{"authorization": "anything"}]})
    assert scan_for_sensitive_data({"name": "AKIAIOSFODNN7EXAMPLE"})
    assert not scan_for_sensitive_data({"name": "秘书", "aria-valuemax": "100", "type": "password"})


@pytest.mark.asyncio
async def test_headed_edge_picker_on_local_static_html(tmp_path):
    if os.getenv("RUN_HEADED_PICKER_INTEGRATION") != "1":
        pytest.skip("需设置 RUN_HEADED_PICKER_INTEGRATION=1 才会打开本机有头 Edge")
    pytest.importorskip("playwright.async_api", reason="未安装 Playwright Python 包")
    from playwright.async_api import async_playwright

    (tmp_path / "index.html").write_text(
        """
        <!doctype html><meta charset="utf-8"><title>Picker Integration</title>
        <button id="normal">普通按钮</button><div id="shadow"></div>
        <input id="identity" type="text" value="11010519491231002X">
        <button id="offscreen" style="margin-top:1600px">屏幕外按钮</button>
        <iframe id="same" src="/frame.html"></iframe>
        <iframe id="cross"></iframe>
        <script>
          window.clickCount = 0;
          window.maskObserved = false;
          normal.addEventListener('click', () => window.clickCount++);
          shadow.attachShadow({mode:'open'}).innerHTML = '<button id="shadow-button">Shadow 按钮</button>';
          new MutationObserver(records => {
            for (const record of records) for (const node of record.addedNodes) {
              if (node.nodeType === 1 && node.hasAttribute('data-synapse-sensitive-mask')) window.maskObserved = true;
            }
          }).observe(document.documentElement, {childList:true, subtree:true});
        </script>
        """,
        encoding="utf-8",
    )
    (tmp_path / "frame.html").write_text(
        "<!doctype html><meta charset='utf-8'><button id='frame-button' style='margin-top:100px'>Frame 按钮</button>"
        "<input id='frame-password' type='password' value='never-visible'>",
        encoding="utf-8",
    )

    class QuietHandler(SimpleHTTPRequestHandler):
        def log_message(self, *_args):
            return

    handler = lambda *args, **kwargs: QuietHandler(*args, directory=str(tmp_path), **kwargs)
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    port = server.server_address[1]
    browser = None
    picker = None
    playwright = await async_playwright().start()
    try:
        try:
            browser = await playwright.chromium.launch(channel="msedge", headless=False)
        except Exception as error:
            pytest.skip(f"本机 Edge 有头浏览器不可用：{type(error).__name__}: {error}")
        context = await browser.new_context()
        picks = []
        captures = []
        captures_by_id = {}

        async def receive_pick(_source, _handle, payload):
            capture = await picker.build_capture(_source, _handle, payload, page.url)
            captures.append(capture)
            picks.append(payload)
            captures_by_id[payload["attributes"].get("id")] = capture

        picker = ElementPicker(rate_limit_per_second=20)
        page = await context.new_page()
        await page.goto(f"http://127.0.0.1:{port}/index.html")
        await picker.install(context, page, receive_pick, page.url)
        await page.evaluate(
            f"document.getElementById('cross').src = 'http://localhost:{port}/frame.html'"
        )
        await page.locator("#normal").hover()
        await page.mouse.click(*await page.locator("#normal").evaluate(
            "e => { const r=e.getBoundingClientRect(); return [r.x+r.width/2,r.y+r.height/2] }"
        ))
        assert await page.evaluate("window.clickCount") == 0
        for _ in range(40):
            if picks:
                break
            await asyncio.sleep(0.05)
        assert picks[-1]["tag"] == "button"

        await page.locator("[data-synapse-capture-host]").locator("button[data-mode=operate]").click()
        await page.locator("#normal").click()
        assert await page.evaluate("window.clickCount") == 1

        await page.keyboard.down("Alt")
        await page.locator("#normal").click(force=True)
        await page.keyboard.up("Alt")
        assert await page.evaluate("window.clickCount") == 1
        for _ in range(40):
            if len(picks) >= 2:
                break
            await asyncio.sleep(0.05)
        assert len(picks) >= 2

        await page.locator("[data-synapse-capture-host]").locator("button[data-mode=pick]").click()
        await page.locator("#shadow").locator("#shadow-button").click(force=True)
        for _ in range(40):
            if len(picks) >= 3:
                break
            await asyncio.sleep(0.05)
        await page.frame_locator("#same").locator("#frame-button").click(force=True)
        for _ in range(40):
            if len(picks) >= 4:
                break
            await asyncio.sleep(0.05)
        await page.locator("#identity").click(force=True)
        for _ in range(40):
            if len(picks) >= 5:
                break
            await asyncio.sleep(0.05)
        assert len(picks) >= 5
        identity_pick = next(item for item in picks if item["attributes"].get("id") == "identity")
        assert "value" not in identity_pick["attributes"]
        assert not await page.evaluate("window.maskObserved")
        assert "11010519491231002X" not in str(captures[-1].candidate.platform_payload())

        await asyncio.sleep(1.05)
        await page.frame_locator("#same").locator("#frame-password").click(force=True)
        await page.locator("#offscreen").click(force=True)
        for _ in range(40):
            if "frame-password" in captures_by_id and "offscreen" in captures_by_id:
                break
            await asyncio.sleep(0.05)
        from PIL import Image

        with Image.open(captures_by_id["frame-password"].screenshot_path) as image:
            center = image.convert("RGB").getpixel((image.width // 2, image.height // 2))
            assert center == (32, 33, 36)
        with Image.open(captures_by_id["offscreen"].screenshot_path) as image:
            assert image.width > 0 and image.height > 0

        normal_handle = await page.locator("#normal").element_handle()
        offscreen_handle = await page.locator("#offscreen").element_handle()
        concurrent_paths = await asyncio.gather(
            picker._screenshot(page, normal_handle),
            picker._screenshot(page, offscreen_handle),
        )
        assert all(path.exists() for path in concurrent_paths)
        for path in concurrent_paths:
            picker._remove_file(path)

        retained = captures_by_id["offscreen"]
        with patch.object(Path, "unlink", side_effect=PermissionError("busy")):
            await picker.remove_capture(retained)
        assert retained.screenshot_path.exists()
        assert await page.evaluate("document.title") == "Picker Integration"

        await page.keyboard.press("Escape")
        assert await page.evaluate(
            "document.querySelector('[data-synapse-capture-host]').shadowRoot.querySelector('[data-highlight]').hidden"
        )
        cross_frame = next(frame for frame in page.frames if frame.url.startswith(f"http://localhost:{port}/"))
        assert await cross_frame.evaluate("typeof globalThis.__synapseCapturePicker") == "undefined"
        assert captures and all(capture.screenshot_path.exists() for capture in captures)
        for capture in captures:
            await picker.remove_capture(capture)
        await picker.dispose(page)
        assert await page.locator("[data-synapse-capture-host]").count() == 0
        assert await page.frame_locator("#same").locator("[data-synapse-capture-host]").count() == 0
    finally:
        if picker:
            await picker.close()
        if browser:
            await browser.close()
        await playwright.stop()
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)
