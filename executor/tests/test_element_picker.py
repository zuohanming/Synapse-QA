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
    def __init__(self, count: int = 1) -> None:
        self._count = count

    async def count(self) -> int:
        return self._count


class FakeFrame:
    def __init__(self, count: int = 1) -> None:
        self.count_value = count
        self.evaluations: list[tuple[object, object]] = []

    def locator(self, _selector: str) -> FakeLocator:
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


class FakeElementHandle:
    def __init__(self, *, screenshot_error: Exception | None = None) -> None:
        self.screenshot_error = screenshot_error
        self.screenshot_paths: list[Path] = []

    async def screenshot(self, *, path: str) -> None:
        if self.screenshot_error:
            raise self.screenshot_error
        target = Path(path)
        target.write_bytes(b"png")
        self.screenshot_paths.append(target)


class FakePickerContext:
    def __init__(self) -> None:
        self.binding = None
        self.scripts = []

    async def expose_binding(self, name, callback, handle=False) -> None:
        self.binding = (name, callback, handle)

    async def add_init_script(self, script: str) -> None:
        self.scripts.append(script)


class FakePickerPage:
    def __init__(self) -> None:
        self.evaluations = []

    async def evaluate(self, expression, argument=None):
        self.evaluations.append((expression, argument))


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

    await picker.install(context, page, lambda *_args: None)
    await picker.set_mode(page, CaptureMode.operate)
    await picker.clear_highlight(page)
    await picker.dispose(page)

    assert context.binding[0] == "__synapseCapturePick"
    assert context.binding[2] is True
    assert len(context.scripts) == 1
    actions = [argument for _, argument in page.evaluations if isinstance(argument, dict)]
    assert actions == [
        {"action": "install"},
        {"action": "mode", "mode": "operate"},
        {"action": "clear"},
        {"action": "dispose"},
    ]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "field",
    ["value", "cookie", "localStorage", "sessionStorage", "outerHTML", "authorization", "selector", "cssSelector", "xpath"],
)
async def test_dom_binding_rejects_every_non_allowlisted_or_sensitive_field(field, tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    payload = safe_payload()
    payload[field] = "must-not-cross-boundary"

    with pytest.raises(PickerSecurityError, match="字段"):
        await picker.build_capture(
            {"frame": FakeFrame()},
            FakeElementHandle(),
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

    capture = await picker.build_capture(
        {"frame": FakeFrame()},
        FakeElementHandle(),
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

    with pytest.raises(PickerSecurityError, match="敏感"):
        await picker.build_capture(
            {"frame": FakeFrame()},
            FakeElementHandle(),
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

    with patch("app.services.element_picker.build_candidate", return_value=unsafe_candidate), patch.object(
        CaptureCandidate, "platform_payload", return_value=unsafe_payload
    ):
        with pytest.raises(PickerSecurityError, match="敏感"):
            await picker.build_capture(
                {"frame": FakeFrame()},
                FakeElementHandle(),
                safe_payload(),
                "https://example.test/orders",
            )

    assert list(tmp_path.iterdir()) == []


@pytest.mark.asyncio
async def test_match_counts_are_recomputed_through_trusted_frame_locators(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)

    capture = await picker.build_capture(
        {"frame": FakeFrame(count=2)},
        FakeElementHandle(),
        safe_payload(locatorMatches={"id:save-order": 1}),
        "https://example.test/orders",
    )

    assert all(not locator.unique for locator in capture.candidate.locators)
    assert all(locator.match_count == 2 for locator in capture.candidate.locators)
    await picker.remove_capture(capture)


@pytest.mark.asyncio
async def test_local_screenshot_masks_sensitive_inputs_and_is_deleted_after_report(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    frame = FakeFrame()
    handle = FakeElementHandle()

    capture = await picker.build_capture(
        {"frame": frame},
        handle,
        safe_payload(),
        "https://example.test/orders",
    )

    assert capture.screenshot_path is not None
    assert capture.screenshot_path.exists()
    assert len(frame.evaluations) == 2
    assert frame.evaluations[0][1] == {"action": "mask"}
    assert frame.evaluations[1][1] == {"action": "unmask"}
    await picker.remove_capture(capture)
    assert not capture.screenshot_path.exists()


@pytest.mark.asyncio
async def test_screenshot_failure_removes_mask_and_partial_temp_file(tmp_path):
    picker = ElementPicker(temp_root=tmp_path)
    frame = FakeFrame()

    with pytest.raises(RuntimeError, match="screenshot failed"):
        await picker.build_capture(
            {"frame": frame},
            FakeElementHandle(screenshot_error=RuntimeError("screenshot failed")),
            safe_payload(),
            "https://example.test/orders",
        )

    assert [argument for _, argument in frame.evaluations] == [{"action": "mask"}, {"action": "unmask"}]
    assert list(tmp_path.rglob("*.png")) == []


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
        "<!doctype html><meta charset='utf-8'><button id='frame-button'>Frame 按钮</button>",
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

        async def receive_pick(_source, _handle, payload):
            capture = await picker.build_capture(_source, _handle, payload, page.url)
            captures.append(capture)
            picks.append(payload)

        picker = ElementPicker()
        page = await context.new_page()
        await picker.install(context, page, receive_pick)
        await page.goto(f"http://127.0.0.1:{port}/index.html")
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

        await picker.set_mode(page, CaptureMode.operate)
        await page.locator("#normal").click()
        assert await page.evaluate("window.clickCount") == 1

        await page.keyboard.down("Alt")
        await page.locator("#normal").click(force=True)
        await page.keyboard.up("Alt")
        assert await page.evaluate("window.clickCount") == 1

        await picker.set_mode(page, CaptureMode.pick)
        await page.locator("#shadow").locator("#shadow-button").click(force=True)
        await page.frame_locator("#same").locator("#frame-button").click(force=True)
        await page.locator("#identity").click(force=True)
        for _ in range(40):
            if len(picks) >= 5:
                break
            await asyncio.sleep(0.05)
        assert len(picks) >= 5
        identity_pick = next(item for item in picks if item["attributes"].get("id") == "identity")
        assert "value" not in identity_pick["attributes"]
        assert await page.evaluate("window.maskObserved")
        assert "11010519491231002X" not in str(captures[-1].candidate.platform_payload())

        await page.keyboard.press("Escape")
        assert await page.evaluate(
            "document.querySelector('[data-synapse-capture-host]').shadowRoot.querySelector('[data-highlight]').hidden"
        )
        await page.wait_for_function(
            "document.querySelector('[data-synapse-capture-host]').shadowRoot.querySelector('[data-message]').textContent.includes('不支持跨域 iframe')"
        )
        cross_frame = next(frame for frame in page.frames if frame.url.startswith(f"http://localhost:{port}/"))
        assert await cross_frame.evaluate("typeof globalThis.__synapseCapturePicker") == "undefined"
        assert captures and all(capture.screenshot_path.exists() for capture in captures)
        for capture in captures:
            await picker.remove_capture(capture)
    finally:
        if picker:
            await picker.close()
        if browser:
            await browser.close()
        await playwright.stop()
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)
