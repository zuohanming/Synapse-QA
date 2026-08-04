import asyncio
import contextlib
import json
import logging
import os
import re
import secrets
import tempfile
import time
from collections import deque
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Awaitable, Callable
from uuid import uuid4

from app.models.capture import CaptureCandidate, CaptureLocator, CaptureMode, ElementSnapshot
from app.services.capture_security import (
    contains_sensitive_data,
    normalize_key,
    same_origin,
    sanitize_public_url,
    url_origin,
)
from app.services.locator_generator import build_candidate


logger = logging.getLogger(__name__)

_BINDING_NAME = "__synapseCapturePick"
_ALLOWED_FIELDS = {
    "tag", "attributes", "accessibleName", "label", "formLabel", "visibleText", "role", "depth", "path", "locatorMatches",
}
_ALLOWED_ATTRIBUTES = {
    "id", "class", "role", "name", "type", "placeholder", "autocomplete", "aria-label", "aria-labelledby",
    "data-testid", "data-test", "data-qa", "data-cy",
}
_FORBIDDEN_KEYS = {
    "value", "cookie", "cookies", "local_storage", "session_storage", "outer_html", "inner_html",
    "authorization", "selector", "css_selector", "xpath", "password", "passwd", "token",
    "access_token", "refresh_token", "api_key", "apikey", "session", "secret", "client_secret",
}
_TAG = re.compile(r"^[a-z][a-z0-9-]{0,63}$")


PICKER_SCRIPT = r"""
options => {
  const nonce = options.nonce;
  const state = {mode: options.mode, alt: false, target: null, disposed: false};
  const resources = [];
  let host = null;

  const safeText = (value, limit) => String(value || "").replace(/\s+/g, " ").trim().slice(0, limit);
  const visibleText = element => {
    if (!element || !element.isConnected) return "";
    const style = getComputedStyle(element);
    const box = element.getBoundingClientRect();
    if (style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse" ||
        Number(style.opacity) === 0 || !box.width || !box.height) return "";
    return safeText(element.innerText, 512);
  };
  const safeAttributes = element => {
    const names = ["id", "class", "role", "name", "type", "placeholder", "autocomplete", "aria-label",
      "aria-labelledby", "data-testid", "data-test", "data-qa", "data-cy"];
    const result = {};
    for (const name of names) {
      const value = safeText(element.getAttribute(name), 256);
      if (value) result[name] = value;
    }
    return result;
  };
  const labelText = element => {
    if (!element.labels) return "";
    return safeText(Array.from(element.labels).map(item => visibleText(item)).join(" "), 512);
  };
  const formLabelText = element => {
    const formItem = element.closest(".el-form-item, fieldset, [role='group']");
    if (!formItem) return "";
    return safeText(visibleText(formItem.querySelector(".el-form-item__label, label, legend, .name")), 512);
  };
  const structuredPath = element => {
    const path = [];
    let current = element;
    while (current && path.length < 32) {
      if (current.nodeType === 1) {
        const siblings = current.parentElement
          ? Array.from(current.parentElement.children).filter(item => item.tagName === current.tagName)
          : [current];
        path.unshift({tag: current.tagName.toLowerCase(), nth: Math.max(1, siblings.indexOf(current) + 1)});
      }
      const parent = current.parentNode;
      current = parent?.nodeType === 11 && parent.host ? parent.host : current.parentElement;
    }
    return path;
  };
  const extract = element => {
    const attributes = safeAttributes(element);
    const text = visibleText(element);
    const label = labelText(element);
    const formLabel = formLabelText(element);
    const path = structuredPath(element);
    return {
      tag: element.tagName.toLowerCase(),
      attributes,
      accessibleName: safeText(attributes["aria-label"] || label || text, 512),
      label,
      formLabel,
      visibleText: text,
      role: safeText(attributes.role, 64),
      depth: path.length,
      path,
      locatorMatches: {}
    };
  };
  const emit = bundle => Promise.resolve(globalThis.__synapseCapturePick({...bundle, nonce})).catch(() => {});
  const effectiveMode = () => state.alt ? (state.mode === "pick" ? "operate" : "pick") : state.mode;

  const clear = () => {
    const highlight = host?.shadowRoot?.querySelector("[data-highlight]");
    if (highlight) highlight.hidden = true;
    state.target = null;
  };
  const setMode = mode => {
    if (mode !== "pick" && mode !== "operate") return;
    state.mode = mode;
    host?.shadowRoot?.querySelectorAll("[data-mode]").forEach(
      button => button.setAttribute("aria-pressed", String(button.getAttribute("data-mode") === mode))
    );
  };
  const eventElement = event => {
    const path = typeof event.composedPath === "function" ? event.composedPath() : [event.target];
    const shadow = host?.shadowRoot;
    if (path.some(item => item === host || item === shadow || item?.getRootNode?.() === shadow)) return null;
    return path.find(item => item?.nodeType === 1) || null;
  };
  const showHighlight = element => {
    const box = element.getBoundingClientRect();
    const highlight = host.shadowRoot.querySelector("[data-highlight]");
    highlight.hidden = false;
    highlight.style.left = `${box.left}px`;
    highlight.style.top = `${box.top}px`;
    highlight.style.width = `${box.width}px`;
    highlight.style.height = `${box.height}px`;
  };
  const install = () => {
    if (state.disposed || host?.isConnected) return;
    host = document.createElement("div");
    host.setAttribute("data-synapse-capture-host", "");
    host.style.cssText = "all:initial;position:fixed;inset:0;z-index:2147483647;pointer-events:none;";
    const shadow = host.attachShadow({mode: "open"});
    shadow.innerHTML = `
      <style>
        :host{all:initial}
        [data-toolbar]{position:fixed;top:14px;left:50%;transform:translateX(-50%);display:flex;gap:6px;
          align-items:center;background:#171717;color:#fff;padding:7px;border-radius:9px;
          font:13px/1.2 system-ui,sans-serif;box-shadow:0 6px 24px #0005;pointer-events:auto}
        button{border:0;border-radius:6px;padding:6px 10px;cursor:pointer}
        button[aria-pressed="true"]{outline:2px solid #8eb0ff}
        [data-highlight]{position:fixed;border:2px solid #5b8cff;background:#5b8cff22;
          box-sizing:border-box;pointer-events:none}
      </style>
      <div data-toolbar>
        <button data-mode="pick">拾取</button>
        <button data-mode="operate">操作</button>
      </div>
      <div data-highlight hidden></div>`;
    shadow.querySelectorAll("[data-mode]").forEach(button => {
      button.addEventListener("click", event => {
        event.preventDefault();
        event.stopImmediatePropagation();
        const mode = button.getAttribute("data-mode");
        setMode(mode);
        emit({action: "mode", value: mode});
      });
    });
    (document.documentElement || document.body).appendChild(host);
    setMode(state.mode);

    const mouseover = event => {
      const element = eventElement(event);
      if (element && effectiveMode() === "pick") {
        state.target = element;
        showHighlight(element);
      }
    };
    const click = event => {
      const element = eventElement(event);
      if (!element || effectiveMode() !== "pick") return;
      event.preventDefault();
      event.stopImmediatePropagation();
      emit({action: "pick", element, payload: extract(element)});
    };
    const keydown = event => {
      if (event.key === "Alt" && !state.alt) {
        state.alt = true;
        emit({action: "alt", value: true});
      }
      if (event.key === "Escape") {
        clear();
        emit({action: "clear"});
      }
    };
    const keyup = event => {
      if (event.key === "Alt" && state.alt) {
        state.alt = false;
        emit({action: "alt", value: false});
      }
    };
    document.addEventListener("mouseover", mouseover, true);
    document.addEventListener("click", click, true);
    document.addEventListener("keydown", keydown, true);
    document.addEventListener("keyup", keyup, true);
    resources.push({mouseover, click, keydown, keyup});
  };
  const dispose = () => {
    if (state.disposed) return;
    state.disposed = true;
    for (const resource of resources.splice(0)) {
      document.removeEventListener("mouseover", resource.mouseover, true);
      document.removeEventListener("click", resource.click, true);
      document.removeEventListener("keydown", resource.keydown, true);
      document.removeEventListener("keyup", resource.keyup, true);
    }
    host?.remove();
    host = null;
  };
  install();
  return command => {
    if (command.action === "mode") setMode(command.mode);
    if (command.action === "alt") state.alt = Boolean(command.value);
    if (command.action === "clear") clear();
    if (command.action === "dispose") dispose();
  };
}
"""

_DOM_EXTRACT_SCRIPT = r"""
element => {
  const safeText = (value, limit) => String(value || "").replace(/\s+/g, " ").trim().slice(0, limit);
  const visibleText = item => {
    if (!item || !item.isConnected) return "";
    const style = getComputedStyle(item);
    const box = item.getBoundingClientRect();
    if (style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse" ||
        Number(style.opacity) === 0 || !box.width || !box.height) return "";
    return safeText(item.innerText, 512);
  };
  const attributes = {};
  for (const name of ["id", "class", "role", "name", "type", "placeholder", "autocomplete", "aria-label",
      "aria-labelledby", "data-testid", "data-test", "data-qa", "data-cy"]) {
    const value = safeText(element.getAttribute(name), 256);
    if (value) attributes[name] = value;
  }
  const label = element.labels
    ? safeText(Array.from(element.labels).map(item => visibleText(item)).join(" "), 512)
    : "";
  const formItem = element.closest(".el-form-item, fieldset, [role='group']");
  const formLabel = formItem
    ? safeText(visibleText(formItem.querySelector(".el-form-item__label, label, legend, .name")), 512)
    : "";
  const path = [];
  let current = element;
  while (current && path.length < 32) {
    if (current.nodeType === 1) {
      const siblings = current.parentElement
        ? Array.from(current.parentElement.children).filter(item => item.tagName === current.tagName)
        : [current];
      path.unshift({tag: current.tagName.toLowerCase(), nth: Math.max(1, siblings.indexOf(current) + 1)});
    }
    const parent = current.parentNode;
    current = parent?.nodeType === 11 && parent.host ? parent.host : current.parentElement;
  }
  const text = visibleText(element);
  return {
    tag: element.tagName.toLowerCase(),
    attributes,
    accessibleName: safeText(attributes["aria-label"] || label || text, 512),
    label,
    formLabel,
    visibleText: text,
    role: safeText(attributes.role, 64),
    depth: path.length,
    path,
    locatorMatches: {}
  };
}
"""

_SENSITIVE_INPUT_BOOLEAN = r"""
input => {
  const type = (input.getAttribute("type") || "").toLowerCase();
  const metadata = ["id", "name", "autocomplete", "aria-label", "placeholder"]
    .map(name => input.getAttribute(name) || "").join(" ");
  const identity = /身份证|identity|id[\s_-]?card|national[\s_-]?id/i;
  const identityValue = /^(?:\d{15}|\d{17}[\dXx])$/;
  return type === "password" || type === "tel" || identity.test(metadata) || identityValue.test(input.value || "");
}
"""


class PickerSecurityError(ValueError):
    pass


@dataclass(frozen=True)
class LocalCapture:
    capture_id: str
    candidate: CaptureCandidate
    screenshot_path: Path | None


class ElementPicker:
    """浏览器拾取脚本和 Python 安全边界。"""

    def __init__(self, temp_root: str | Path | None = None, *, rate_limit_per_second: int = 5) -> None:
        self.script = PICKER_SCRIPT
        self._temp_root = Path(temp_root) if temp_root is not None else None
        self._session_temp_dir: Path | None = None
        self._local_files: set[Path] = set()
        self._retry_files: set[Path] = set()
        self._nonce = ""
        self._page: Any = None
        self._capture_origin = ""
        self._callback: Callable[..., Awaitable[None]] | None = None
        self._controllers: dict[int, tuple[Any, str, Any]] = {}
        self._mode = CaptureMode.pick
        self._rate_limit = max(1, rate_limit_per_second)
        self._pick_times: deque[float] = deque()
        self._pick_lock = asyncio.Semaphore(1)
        self._screenshot_lock = asyncio.Lock()
        self._event_tasks: set[asyncio.Task[Any]] = set()
        self._binding_tasks: set[asyncio.Task[Any]] = set()

    async def install(
        self,
        context: Any,
        page: Any,
        callback: Callable[..., Awaitable[None]],
        capture_url: str | None = None,
    ) -> None:
        self._nonce = secrets.token_urlsafe(32)
        self._page = page
        self._callback = callback
        self._capture_origin = url_origin(capture_url or page.url)

        async def receive_bundle(source: dict[str, Any], bundle_handle: Any) -> None:
            await self._receive_bundle(source, bundle_handle)

        await context.expose_binding(_BINDING_NAME, receive_bundle, handle=True)
        if hasattr(page, "on"):
            page.on("framenavigated", self._on_frame_navigated)
            page.on("frameattached", self._on_frame_navigated)
        await self.install_on_page(page)

    def _on_frame_navigated(self, frame: Any) -> None:
        task = asyncio.create_task(self._install_frame(frame))
        self._event_tasks.add(task)
        task.add_done_callback(self._event_tasks.discard)

    async def install_on_page(self, page: Any) -> None:
        frames = list(getattr(page, "frames", []) or [])
        if not frames:
            frames = [page]
        for frame in frames:
            await self._install_frame(frame)

    async def _install_frame(self, frame: Any) -> None:
        if not self._trusted_frame(frame):
            return
        frame_url = str(getattr(frame, "url", ""))
        key = id(frame)
        existing = self._controllers.get(key)
        if existing and existing[0] is frame and existing[1] == frame_url:
            return
        if existing:
            await _safe_handle_dispose(existing[2])
        controller = await frame.evaluate_handle(
            self.script,
            {"nonce": self._nonce, "mode": self._mode.value},
        )
        self._controllers[key] = (frame, frame_url, controller)

    async def set_mode(self, page: Any, mode: CaptureMode) -> None:
        del page
        self._mode = mode
        await self._broadcast({"action": "mode", "mode": mode.value})

    async def clear_highlight(self, page: Any) -> None:
        del page
        await self._broadcast({"action": "clear"})

    async def dispose(self, page: Any) -> None:
        del page
        await self._drain_binding_tasks()
        await self._broadcast({"action": "dispose"})
        for _, _, handle in tuple(self._controllers.values()):
            await _safe_handle_dispose(handle)
        self._controllers.clear()

    async def _broadcast(self, command: dict[str, Any]) -> None:
        for key, (frame, _, controller) in tuple(self._controllers.items()):
            if not self._trusted_frame(frame):
                await _safe_handle_dispose(controller)
                self._controllers.pop(key, None)
                continue
            with contextlib.suppress(Exception):
                await controller.evaluate("(control, value) => control(value)", command)

    async def _receive_bundle(self, source: dict[str, Any], bundle_handle: Any) -> None:
        current_task = asyncio.current_task()
        if current_task is not None:
            self._binding_tasks.add(current_task)
        handles: list[Any] = [bundle_handle]
        try:
            nonce = await _handle_json_property(bundle_handle, "nonce", handles)
            action = await _handle_json_property(bundle_handle, "action", handles)
            if not isinstance(nonce, str) or not secrets.compare_digest(nonce, self._nonce):
                raise PickerSecurityError("候选 nonce 无效")
            frame = await self._validate_source(source)
            if action in {"mode", "alt", "clear"}:
                await self._handle_control_action(action, bundle_handle, handles)
                return
            if action != "pick" or not self._accept_rate():
                return
            async with self._pick_lock:
                element_property = await bundle_handle.get_property("element")
                handles.append(element_property)
                element_handle = element_property.as_element()
                payload = await _handle_json_property(bundle_handle, "payload", handles)
                if element_handle is None or not isinstance(payload, dict):
                    raise PickerSecurityError("候选来源无效")
                owner_frame = await element_handle.owner_frame()
                if owner_frame is not frame:
                    raise PickerSecurityError("候选元素不属于当前 frame")
                trusted_payload = await element_handle.evaluate(_DOM_EXTRACT_SCRIPT)
                if _sanitize_binding_payload(payload) != _sanitize_binding_payload(trusted_payload):
                    raise PickerSecurityError("候选 DOM 复核不一致")
                if self._callback:
                    await self._callback(source, element_handle, trusted_payload)
        except PickerSecurityError:
            logger.warning("页面元素 binding 因安全边界被丢弃")
        finally:
            for handle in reversed(handles):
                await _safe_handle_dispose(handle)
            if current_task is not None:
                self._binding_tasks.discard(current_task)

    async def _handle_control_action(self, action: str, bundle_handle: Any, handles: list[Any]) -> None:
        if action == "mode":
            value = await _handle_json_property(bundle_handle, "value", handles)
            if value in {"pick", "operate"}:
                self._mode = CaptureMode(value)
                await self._broadcast({"action": "mode", "mode": value})
        elif action == "alt":
            value = await _handle_json_property(bundle_handle, "value", handles)
            await self._broadcast({"action": "alt", "value": bool(value)})
        else:
            await self._broadcast({"action": "clear"})

    async def _validate_source(self, source: Any) -> Any:
        if not isinstance(source, dict) or source.get("page") is not self._page:
            raise PickerSecurityError("候选 page 来源无效")
        frame = source.get("frame")
        if frame is None or not self._trusted_frame(frame):
            raise PickerSecurityError("候选 frame 来源无效")
        frame_page = getattr(frame, "page", None)
        if callable(frame_page):
            frame_page = frame_page()
        if frame_page is not None and frame_page is not self._page:
            raise PickerSecurityError("候选 frame 不属于当前页面")
        return frame

    def _trusted_frame(self, frame: Any) -> bool:
        frame_url = str(getattr(frame, "url", ""))
        if not frame_url or not same_origin(frame_url, self._capture_origin):
            return False
        current = frame
        while current is not None:
            if not same_origin(str(getattr(current, "url", "")), self._capture_origin):
                return False
            current = getattr(current, "parent_frame", None)
            if callable(current):
                current = current()
        return True

    def _accept_rate(self) -> bool:
        now = time.monotonic()
        while self._pick_times and self._pick_times[0] <= now - 1:
            self._pick_times.popleft()
        if len(self._pick_times) >= self._rate_limit:
            return False
        self._pick_times.append(now)
        return True

    async def build_capture(
        self,
        source: dict[str, Any],
        element_handle: Any,
        raw_payload: dict[str, Any],
        capture_url: str,
    ) -> LocalCapture:
        try:
            payload = _sanitize_binding_payload(raw_payload)
            if contains_sensitive_data(payload):
                raise PickerSecurityError("候选包含敏感数据，已丢弃")
            snapshot = ElementSnapshot(
                tag=payload["tag"],
                attributes=payload["attributes"],
                accessible_name=payload["accessibleName"],
                label=payload["label"],
                form_label=payload["formLabel"],
                visible_text=payload["visibleText"],
                depth=payload["depth"],
                capture_url=sanitize_public_url(capture_url),
            )
            initial = build_candidate(snapshot)
            frame = source.get("frame") if isinstance(source, dict) else None
            page = source.get("page") if isinstance(source, dict) else None
            if frame is None:
                raise PickerSecurityError("候选来源无效")
            matches: dict[str, int] = {}
            for locator in initial.locators:
                matches[f"{locator.type}:{locator.value}"] = await _count_locator(frame, locator)
            candidate = build_candidate(snapshot.model_copy(update={"locator_matches": matches}))
            if contains_sensitive_data(candidate.platform_payload()):
                raise PickerSecurityError("候选包含敏感数据，已丢弃")
            screenshot_path = await self._screenshot(page or self._page, element_handle)
            return LocalCapture(str(uuid4()), candidate, screenshot_path)
        except PickerSecurityError:
            logger.warning("页面元素候选因安全边界被丢弃")
            raise

    async def remove_capture(self, capture: LocalCapture) -> None:
        self._remove_file(capture.screenshot_path)
        self._remove_empty_temp_dir()

    async def close(self) -> None:
        await self._drain_binding_tasks()
        for task in tuple(self._event_tasks):
            task.cancel()
        if self._event_tasks:
            await asyncio.gather(*self._event_tasks, return_exceptions=True)
        for path in tuple(self._local_files | self._retry_files):
            self._remove_file(path)
        self._remove_empty_temp_dir()
        self._nonce = ""
        self._callback = None
        self._page = None

    async def _drain_binding_tasks(self) -> None:
        current = asyncio.current_task()
        pending = [task for task in self._binding_tasks if task is not current and not task.done()]
        if pending:
            await asyncio.gather(*pending, return_exceptions=True)

    async def _screenshot(self, page: Any, element_handle: Any) -> Path:
        if page is None:
            raise PickerSecurityError("候选页面来源无效")
        async with self._screenshot_lock:
            path = self._new_screenshot_path()
            self._local_files.add(path)
            try:
                await element_handle.scroll_into_view_if_needed()
                box = await element_handle.bounding_box()
                if not isinstance(box, dict) or box.get("width", 0) <= 0 or box.get("height", 0) <= 0:
                    raise PickerSecurityError("候选元素不可见")
                masks = await self._sensitive_mask_locators(page)
                await page.screenshot(
                    path=str(path),
                    clip={key: float(box[key]) for key in ("x", "y", "width", "height")},
                    mask=masks,
                    mask_color="#202124",
                    animations="disabled",
                )
                if os.name != "nt":
                    with contextlib.suppress(OSError):
                        os.chmod(path, 0o600)
                return path
            except Exception:
                self._remove_file(path)
                self._remove_empty_temp_dir()
                raise

    async def _sensitive_mask_locators(self, page: Any) -> list[Any]:
        masks: list[Any] = []
        sensitive_child_frame = False
        for frame in list(getattr(page, "frames", []) or []):
            if not self._trusted_frame(frame):
                continue
            typed = frame.locator('input[type="password"], input[type="tel"]')
            frame_has_sensitive = bool(await typed.count())
            if frame_has_sensitive:
                masks.append(typed)
            inputs = frame.locator("input")
            for index in range(min(await inputs.count(), 1000)):
                item = inputs.nth(index)
                if await item.evaluate(_SENSITIVE_INPUT_BOOLEAN):
                    masks.append(item)
                    frame_has_sensitive = True
            if frame_has_sensitive and getattr(frame, "parent_frame", None) is not None:
                sensitive_child_frame = True
        if sensitive_child_frame:
            # Chromium 对直接来自 Frame 的 mask locator 在部分版本中不会投影到主页面截图；
            # 保守遮住 iframe 视口，避免同源 frame 内的敏感像素漏出。
            masks.append(page.locator("iframe"))
        return masks

    def _new_screenshot_path(self) -> Path:
        if self._session_temp_dir is None:
            parent = str(self._temp_root) if self._temp_root is not None else None
            self._session_temp_dir = Path(tempfile.mkdtemp(prefix="synapse-capture-", dir=parent))
            if os.name != "nt":
                with contextlib.suppress(OSError):
                    os.chmod(self._session_temp_dir, 0o700)
        return self._session_temp_dir / f"{uuid4().hex}.png"

    def _remove_file(self, path: Path | None) -> None:
        if path is None:
            return
        try:
            path.unlink(missing_ok=True)
        except OSError:
            self._retry_files.add(path)
            return
        self._retry_files.discard(path)
        self._local_files.discard(path)

    def _remove_empty_temp_dir(self) -> None:
        if self._session_temp_dir and self._session_temp_dir.exists() and not self._retry_files:
            try:
                self._session_temp_dir.rmdir()
                self._session_temp_dir = None
            except OSError:
                pass


scan_for_sensitive_data = contains_sensitive_data


async def _handle_json_property(bundle_handle: Any, name: str, handles: list[Any]) -> Any:
    handle = await bundle_handle.get_property(name)
    handles.append(handle)
    return await handle.json_value()


async def _safe_handle_dispose(handle: Any) -> None:
    if handle is None:
        return
    with contextlib.suppress(Exception):
        await handle.dispose()


def _sanitize_binding_payload(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict) or set(raw) - _ALLOWED_FIELDS:
        raise PickerSecurityError("候选包含非允许字段")
    tag = str(raw.get("tag", "")).strip().lower()
    if not _TAG.fullmatch(tag):
        raise PickerSecurityError("候选标签无效")
    raw_attributes = raw.get("attributes", {})
    if not isinstance(raw_attributes, dict):
        raise PickerSecurityError("候选属性结构无效")
    attributes: dict[str, str] = {}
    for raw_key, raw_value in raw_attributes.items():
        key = str(raw_key).strip().lower()
        if normalize_key(key) in _FORBIDDEN_KEYS:
            raise PickerSecurityError("候选包含非允许字段")
        if key not in _ALLOWED_ATTRIBUTES:
            continue
        if not isinstance(raw_value, str):
            raise PickerSecurityError("候选属性值无效")
        value = " ".join(raw_value.split())[:256]
        if value:
            attributes[key] = value
    role = _safe_scalar(raw.get("role", ""), 64)
    if role and "role" not in attributes:
        attributes["role"] = role
    depth = raw.get("depth", 0)
    if isinstance(depth, bool) or not isinstance(depth, int) or not 0 <= depth <= 256:
        raise PickerSecurityError("候选深度无效")
    path = raw.get("path", [])
    _validate_path(path)
    if not isinstance(raw.get("locatorMatches", {}), dict):
        raise PickerSecurityError("候选匹配数结构无效")
    return {
        "tag": tag,
        "attributes": attributes,
        "accessibleName": _safe_scalar(raw.get("accessibleName", ""), 512),
        "label": _safe_scalar(raw.get("label", ""), 512),
        "formLabel": _safe_scalar(raw.get("formLabel", ""), 512),
        "visibleText": _safe_scalar(raw.get("visibleText", ""), 512),
        "role": role,
        "depth": depth,
        "path": path,
        "locatorMatches": {},
    }


def _validate_path(value: Any) -> None:
    if not isinstance(value, list) or len(value) > 32:
        raise PickerSecurityError("候选结构路径无效")
    for segment in value:
        if not isinstance(segment, dict) or set(segment) != {"tag", "nth"}:
            raise PickerSecurityError("候选结构路径无效")
        tag = str(segment.get("tag", "")).lower()
        nth = segment.get("nth")
        if not _TAG.fullmatch(tag) or isinstance(nth, bool) or not isinstance(nth, int) or not 1 <= nth <= 10000:
            raise PickerSecurityError("候选结构路径无效")


def _safe_scalar(value: Any, limit: int) -> str:
    if not isinstance(value, str):
        raise PickerSecurityError("候选字段值无效")
    return " ".join(value.split())[:limit]


async def _count_locator(frame: Any, locator: CaptureLocator) -> int:
    if locator.type == "testid":
        target = frame.get_by_test_id(locator.value)
    elif locator.type == "id":
        target = frame.locator(f'[id={_css_string(locator.value)}]')
    elif locator.type == "role":
        match = re.fullmatch(r"([A-Za-z][A-Za-z0-9-]*)\[name=(.+)]", locator.value)
        if not match:
            return 0
        try:
            name = json.loads(match.group(2))
        except json.JSONDecodeError:
            return 0
        target = frame.get_by_role(match.group(1), name=name)
    elif locator.type == "label":
        target = frame.get_by_label(locator.value, exact=True)
    elif locator.type == "text":
        target = frame.get_by_text(locator.value, exact=True)
    elif locator.type == "xpath":
        target = frame.locator(f"xpath={locator.value}")
    else:
        target = frame.locator(locator.value)
    count = await target.count()
    if isinstance(count, bool) or not isinstance(count, int) or count < 0:
        return 0
    return min(count, 100_000)


def _css_string(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'
