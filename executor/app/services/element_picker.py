import json
import logging
import math
import os
import re
import shutil
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Awaitable, Callable
from uuid import uuid4

from app.models.capture import CaptureCandidate, CaptureLocator, CaptureMode, ElementSnapshot
from app.services.locator_generator import build_candidate


logger = logging.getLogger(__name__)

_BINDING_NAME = "__synapseCapturePick"
_ALLOWED_FIELDS = {
    "tag", "attributes", "accessibleName", "label", "visibleText", "role", "depth", "path", "locatorMatches",
}
_ALLOWED_ATTRIBUTES = {
    "id", "class", "role", "name", "type", "placeholder", "aria-label", "aria-labelledby",
    "data-testid", "data-test", "data-qa", "data-cy",
}
_FORBIDDEN_KEYS = {
    "value", "cookie", "cookies", "localstorage", "sessionstorage", "outerhtml", "innerhtml",
    "authorization", "selector", "cssselector", "xpath", "password", "passwd", "token",
    "access_token", "refresh_token", "api_key", "apikey", "session", "secret", "client_secret",
}
_SECRET_MARKER = re.compile(
    r"(?<![a-z0-9])(?:access[_ -]?token|refresh[_ -]?token|api[_ -]?key|client[_ -]?secret|"
    r"token|secret|cookie|authorization|bearer)(?![a-z0-9])",
    re.IGNORECASE,
)
_JWT = re.compile(r"^eyJ[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{6,}$")
_COMMON_SECRET = re.compile(
    r"^(?:(?:sk|pk|ghp|github_pat|xox[baprs]|AIza)[_-][A-Za-z0-9_-]{12,}|AKIA[A-Z0-9]{12,})$",
    re.IGNORECASE,
)
_TAG = re.compile(r"^[a-z][a-z0-9-]{0,63}$")


PICKER_SCRIPT = r"""
(() => {
  try {
    if (globalThis.top !== globalThis && globalThis.top.location.origin !== globalThis.location.origin) return;
  } catch (_) {
    return;
  }
  if (globalThis.__synapseCapturePicker) {
    globalThis.__synapseCapturePicker.install();
    return;
  }
  const installedDocuments = new WeakSet();
  const pendingDocuments = new WeakSet();
  const observedFrames = new WeakSet();
  const resources = [];
  const state = { mode: "pick", alt: false, target: null };

  function effectiveMode() {
    return state.alt ? (state.mode === "pick" ? "operate" : "pick") : state.mode;
  }

  function safeText(value, limit) {
    return String(value || "").replace(/\s+/g, " ").trim().slice(0, limit);
  }

  function toolbarFor(doc) {
    let host = doc.querySelector("[data-synapse-capture-host]");
    if (host) return host;
    host = doc.createElement("div");
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
        [data-message]{max-width:300px;color:#ffd48a}
        [data-highlight]{position:fixed;border:2px solid #5b8cff;background:#5b8cff22;
          box-sizing:border-box;pointer-events:none}
      </style>
      <div data-toolbar>
        <button data-mode="pick">拾取</button>
        <button data-mode="operate">操作</button>
        <span data-message></span>
      </div>
      <div data-highlight hidden></div>`;
    shadow.querySelectorAll("[data-mode]").forEach(button => {
      button.addEventListener("click", event => {
        event.preventDefault();
        event.stopPropagation();
        state.mode = button.getAttribute("data-mode");
      });
    });
    (doc.documentElement || doc.body).appendChild(host);
    return host;
  }

  function message(doc, text) {
    const host = toolbarFor(doc);
    host.shadowRoot.querySelector("[data-message]").textContent = text;
  }

  function clear(doc) {
    const host = doc.querySelector("[data-synapse-capture-host]");
    if (host) host.shadowRoot.querySelector("[data-highlight]").hidden = true;
    state.target = null;
  }

  function clearAll() {
    for (const resource of resources) clear(resource.doc);
  }

  function showHighlight(doc, element) {
    const host = toolbarFor(doc);
    const box = element.getBoundingClientRect();
    const highlight = host.shadowRoot.querySelector("[data-highlight]");
    highlight.hidden = false;
    highlight.style.left = `${box.left}px`;
    highlight.style.top = `${box.top}px`;
    highlight.style.width = `${box.width}px`;
    highlight.style.height = `${box.height}px`;
  }

  function eventElement(event) {
    const path = typeof event.composedPath === "function" ? event.composedPath() : [event.target];
    return path.find(item => item?.nodeType === 1 && !item.closest?.("[data-synapse-capture-host]")) || null;
  }

  function safeAttributes(element) {
    const names = ["id", "class", "role", "name", "type", "placeholder", "aria-label",
      "aria-labelledby", "data-testid", "data-test", "data-qa", "data-cy"];
    const result = {};
    for (const name of names) {
      const value = safeText(element.getAttribute(name), 256);
      if (value) result[name] = value;
    }
    return result;
  }

  function labelText(element) {
    if (!element.labels) return "";
    return safeText(Array.from(element.labels).map(item => item.textContent).join(" "), 512);
  }

  function structuredPath(element) {
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
  }

  function payload(element) {
    const attributes = safeAttributes(element);
    const accessibleName = safeText(attributes["aria-label"] || labelText(element) || element.textContent, 512);
    return {
      tag: element.tagName.toLowerCase(),
      attributes,
      accessibleName,
      label: labelText(element),
      visibleText: safeText(element.textContent, 512),
      role: safeText(attributes.role, 64),
      depth: structuredPath(element).length,
      path: structuredPath(element),
      locatorMatches: {}
    };
  }

  function scanFrames(doc) {
    for (const frame of Array.from(doc.querySelectorAll("iframe"))) {
      if (!observedFrames.has(frame)) {
        observedFrames.add(frame);
        frame.addEventListener("load", () => scanFrames(doc));
      }
      try {
        const childDocument = frame.contentDocument;
        if (!childDocument) {
          message(doc, "不支持跨域 iframe");
        } else {
          installDocument(childDocument);
        }
      } catch (_) {
        message(doc, "不支持跨域 iframe");
      }
    }
  }

  function installDocument(doc) {
    if (!doc || installedDocuments.has(doc)) return;
    if (doc.readyState === "loading" || (!doc.documentElement && !doc.body)) {
      if (!pendingDocuments.has(doc)) {
        pendingDocuments.add(doc);
        doc.addEventListener("DOMContentLoaded", () => {
          pendingDocuments.delete(doc);
          installDocument(doc);
        }, {once: true});
      }
      return;
    }
    toolbarFor(doc);
    const mouseover = event => {
      const element = eventElement(event);
      if (element && effectiveMode() === "pick") {
        state.target = element;
        showHighlight(doc, element);
      }
    };
    const click = event => {
      const element = eventElement(event);
      if (!element || effectiveMode() !== "pick") return;
      event.preventDefault();
      event.stopImmediatePropagation();
      Promise.resolve(globalThis.__synapseCapturePick({element, payload: payload(element)})).catch(() => {});
    };
    const keydown = event => {
      if (event.key === "Alt") state.alt = true;
      if (event.key === "Escape") clearAll();
    };
    const keyup = event => {
      if (event.key === "Alt") state.alt = false;
    };
    doc.addEventListener("mouseover", mouseover, true);
    doc.addEventListener("click", click, true);
    doc.addEventListener("keydown", keydown, true);
    doc.addEventListener("keyup", keyup, true);
    const observer = new MutationObserver(() => scanFrames(doc));
    observer.observe(doc.documentElement, {childList: true, subtree: true});
    resources.push({doc, mouseover, click, keydown, keyup, observer});
    installedDocuments.add(doc);
    scanFrames(doc);
  }

  function dispose() {
    for (const resource of resources.splice(0)) {
      resource.doc.removeEventListener("mouseover", resource.mouseover, true);
      resource.doc.removeEventListener("click", resource.click, true);
      resource.doc.removeEventListener("keydown", resource.keydown, true);
      resource.doc.removeEventListener("keyup", resource.keyup, true);
      resource.observer.disconnect();
      resource.doc.querySelector("[data-synapse-capture-host]")?.remove();
    }
    delete globalThis.__synapseCapturePicker;
  }

  globalThis.__synapseCapturePicker = {
    install: () => installDocument(document),
    setMode: mode => { if (mode === "pick" || mode === "operate") state.mode = mode; },
    clear: clearAll,
    dispose
  };
  installDocument(document);
})();
"""

_CONTROL_SCRIPT = r"""
command => {
  const picker = globalThis.__synapseCapturePicker;
  if (!picker) return;
  if (command.action === "install") picker.install();
  if (command.action === "mode") picker.setMode(command.mode);
  if (command.action === "clear") picker.clear();
  if (command.action === "dispose") picker.dispose();
}
"""

_MASK_SCRIPT = r"""
command => {
  const marker = "data-synapse-sensitive-mask";
  document.querySelectorAll(`[${marker}]`).forEach(item => item.remove());
  if (command.action !== "mask") return;
  const host = document.createElement("div");
  host.setAttribute(marker, "");
  host.style.cssText = "all:initial;position:fixed;inset:0;z-index:2147483646;pointer-events:none;";
  const shadow = host.attachShadow({mode:"open"});
  const identity = /身份证|identity|id[\s_-]?card|national[\s_-]?id/i;
  const identityValue = /^(?:\d{15}|\d{17}[\dXx])$/;
  const roots = [document];
  const inputs = [];
  while (roots.length) {
    const root = roots.pop();
    for (const element of root.querySelectorAll("*")) {
      if (element.shadowRoot) roots.push(element.shadowRoot);
      if (element.tagName === "INPUT") inputs.push(element);
    }
  }
  for (const input of inputs) {
    const type = (input.getAttribute("type") || "").toLowerCase();
    const metadata = ["id","name","autocomplete","aria-label","placeholder"]
      .map(name => input.getAttribute(name) || "").join(" ");
    // value 仅在页面内用于布尔模式判断，绝不离开浏览器上下文。
    if (type !== "password" && type !== "tel" && !identity.test(metadata) &&
        !identityValue.test(input.value || "")) continue;
    const box = input.getBoundingClientRect();
    if (!box.width || !box.height) continue;
    const mask = document.createElement("span");
    mask.style.cssText = `position:fixed;left:${box.left}px;top:${box.top}px;width:${box.width}px;` +
      `height:${box.height}px;background:#202124;`;
    shadow.appendChild(mask);
  }
  (document.documentElement || document.body).appendChild(host);
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

    def __init__(self, temp_root: str | Path | None = None) -> None:
        self.script = PICKER_SCRIPT
        self._temp_root = Path(temp_root) if temp_root is not None else None
        self._session_temp_dir: Path | None = None
        self._local_files: set[Path] = set()

    async def install(self, context: Any, page: Any, callback: Callable[..., Awaitable[None]]) -> None:
        async def receive_bundle(source: dict[str, Any], bundle_handle: Any) -> None:
            element_property = payload_property = None
            try:
                element_property = await bundle_handle.get_property("element")
                payload_property = await bundle_handle.get_property("payload")
                element_handle = element_property.as_element()
                payload = await payload_property.json_value()
                if element_handle is None or not isinstance(payload, dict):
                    raise PickerSecurityError("候选来源无效")
                await callback(source, element_handle, payload)
            finally:
                for handle in (payload_property, element_property, bundle_handle):
                    if handle is not None:
                        await handle.dispose()

        await context.expose_binding(_BINDING_NAME, receive_bundle, handle=True)
        await context.add_init_script(self.script)
        await self.install_on_page(page)

    async def install_on_page(self, page: Any) -> None:
        await page.evaluate(self.script)
        await page.evaluate(_CONTROL_SCRIPT, {"action": "install"})

    async def set_mode(self, page: Any, mode: CaptureMode) -> None:
        await page.evaluate(_CONTROL_SCRIPT, {"action": "mode", "mode": mode.value})

    async def clear_highlight(self, page: Any) -> None:
        await page.evaluate(_CONTROL_SCRIPT, {"action": "clear"})

    async def dispose(self, page: Any) -> None:
        await page.evaluate(_CONTROL_SCRIPT, {"action": "dispose"})

    async def build_capture(
        self,
        source: dict[str, Any],
        element_handle: Any,
        raw_payload: dict[str, Any],
        capture_url: str,
    ) -> LocalCapture:
        try:
            payload = _sanitize_binding_payload(raw_payload)
            if scan_for_sensitive_data(payload):
                raise PickerSecurityError("候选包含敏感数据，已丢弃")
            snapshot = ElementSnapshot(
                tag=payload["tag"],
                attributes=payload["attributes"],
                accessible_name=payload["accessibleName"],
                label=payload["label"],
                visible_text=payload["visibleText"],
                depth=payload["depth"],
                capture_url=capture_url,
            )
            initial = build_candidate(snapshot)
            frame = source.get("frame") if isinstance(source, dict) else None
            if frame is None:
                raise PickerSecurityError("候选来源无效")
            matches: dict[str, int] = {}
            for locator in initial.locators:
                count = await _count_locator(frame, locator)
                matches[f"{locator.type}:{locator.value}"] = count
            candidate = build_candidate(snapshot.model_copy(update={"locator_matches": matches}))
            if scan_for_sensitive_data(candidate.platform_payload()):
                raise PickerSecurityError("候选包含敏感数据，已丢弃")
            screenshot_path = await self._screenshot(frame, element_handle)
            return LocalCapture(str(uuid4()), candidate, screenshot_path)
        except PickerSecurityError:
            logger.warning("页面元素候选因安全边界被丢弃")
            raise

    async def remove_capture(self, capture: LocalCapture) -> None:
        self._remove_file(capture.screenshot_path)
        self._remove_empty_temp_dir()

    async def close(self) -> None:
        for path in tuple(self._local_files):
            self._remove_file(path)
        if self._session_temp_dir:
            shutil.rmtree(self._session_temp_dir, ignore_errors=True)
            self._session_temp_dir = None

    async def _screenshot(self, frame: Any, element_handle: Any) -> Path:
        path = self._new_screenshot_path()
        try:
            await frame.evaluate(_MASK_SCRIPT, {"action": "mask"})
            await element_handle.screenshot(path=str(path))
            try:
                os.chmod(path, 0o600)
            except OSError:
                pass
            self._local_files.add(path)
            return path
        except Exception:
            self._remove_file(path)
            self._remove_empty_temp_dir()
            raise
        finally:
            await frame.evaluate(_MASK_SCRIPT, {"action": "unmask"})

    def _new_screenshot_path(self) -> Path:
        if self._session_temp_dir is None:
            parent = str(self._temp_root) if self._temp_root is not None else None
            self._session_temp_dir = Path(tempfile.mkdtemp(prefix="synapse-capture-", dir=parent))
            try:
                os.chmod(self._session_temp_dir, 0o700)
            except OSError:
                pass
        return self._session_temp_dir / f"{uuid4().hex}.png"

    def _remove_file(self, path: Path | None) -> None:
        if path is None:
            return
        try:
            path.unlink(missing_ok=True)
        finally:
            self._local_files.discard(path)

    def _remove_empty_temp_dir(self) -> None:
        if self._session_temp_dir and self._session_temp_dir.exists():
            try:
                self._session_temp_dir.rmdir()
                self._session_temp_dir = None
            except OSError:
                pass


def scan_for_sensitive_data(value: Any, key: str = "") -> bool:
    normalized_key = _normalized_key(key)
    if normalized_key in _FORBIDDEN_KEYS:
        return True
    if isinstance(value, dict):
        locator_payload = (
            "type" in value
            and "value" in value
            and set(value).issubset({"type", "value", "score", "unique", "matchCount"})
        )
        return any(
            scan_for_sensitive_data(item, "locator_value" if locator_payload and item_key == "value" else str(item_key))
            for item_key, item in value.items()
        )
    if isinstance(value, (list, tuple)):
        return any(scan_for_sensitive_data(item, key) for item in value)
    if not isinstance(value, str):
        return False
    text = " ".join(value.split())
    if not text or (normalized_key == "type" and text.lower() in {"password", "tel"}):
        return False
    if normalized_key == "fingerprint" and re.fullmatch(r"[a-f0-9]{64}", text):
        return False
    return bool(
        _SECRET_MARKER.search(text)
        or _JWT.fullmatch(text)
        or _COMMON_SECRET.fullmatch(text)
        or _looks_high_entropy_token(text)
    )


def _sanitize_binding_payload(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict):
        raise PickerSecurityError("候选字段结构无效")
    unknown = {_normalized_key(key) for key in raw if key not in _ALLOWED_FIELDS}
    if unknown:
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
        normalized = _normalized_key(key)
        if normalized in _FORBIDDEN_KEYS:
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
    _validate_path(raw.get("path", []))
    if not isinstance(raw.get("locatorMatches", {}), dict):
        raise PickerSecurityError("候选匹配数结构无效")
    return {
        "tag": tag,
        "attributes": attributes,
        "accessibleName": _safe_scalar(raw.get("accessibleName", ""), 512),
        "label": _safe_scalar(raw.get("label", ""), 512),
        "visibleText": _safe_scalar(raw.get("visibleText", ""), 512),
        "role": role,
        "depth": depth,
        "path": raw.get("path", []),
        # 网页提供的匹配数仅属于入站 allowlist；可信 Python 会重新计算，不使用该值。
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


def _normalized_key(value: str) -> str:
    return re.sub(r"[-_.:/\\\s]+", "_", str(value).strip().lower())


def _looks_high_entropy_token(value: str) -> bool:
    if len(value) < 24 or not re.fullmatch(r"[A-Za-z0-9_+/=-]+", value):
        return False
    entropy = -sum(
        (value.count(char) / len(value)) * math.log2(value.count(char) / len(value))
        for char in set(value)
    )
    return entropy >= 3.5
