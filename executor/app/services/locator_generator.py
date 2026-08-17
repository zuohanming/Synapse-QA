import hashlib
import json
import math
import re
import unicodedata
from dataclasses import dataclass

from app.models.capture import CaptureCandidate, CaptureLocator, ElementSnapshot
from app.services.capture_security import contains_secret_text, is_sensitive_key, sanitize_public_url


_SCORES = {"testid": 95, "id": 90, "framework": 90, "row-context": 90, "role": 85, "form-label": 95, "label": 82, "attribute": 82, "stable-xpath": 80, "row-index": 55, "css": 70, "text": 60, "xpath": 40}
_PRIORITY = {strategy: index for index, strategy in enumerate(_SCORES)}
_UUID = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", re.IGNORECASE)
_CSS_HASH = re.compile(r"^(?:css|sc|emotion|jss|mui)-[a-z0-9_-]{5,}$", re.IGNORECASE)
_CSS_MODULE = re.compile(r"^[A-Za-z][A-Za-z0-9_-]*__[A-Za-z0-9_-]*[0-9A-Z][A-Za-z0-9_-]{3,}$")
_REACT_ID = re.compile(r"^:r[0-9a-z]+:$", re.IGNORECASE)
_CONTROL = re.compile(r"[\x00-\x1f\x7f]")
_XPATH_SENSITIVE_ATTRIBUTE = re.compile(r"@\s*(?:value|password|passwd|token|access_token|refresh_token|api[-_]?key|apikey|session|cookie|authorization|secret|client_secret)\b", re.IGNORECASE)
_STABLE_XPATH = re.compile(r"^//[a-z][a-z0-9-]*\[@(?:data-testid|id|name|aria-label|placeholder)=.+\]$|^//(?:button|a)\[normalize-space\(\.\)=.+\]$|^//div\[not\(contains\(@style, 'display: none'\)\)\]//li\[normalize-space\(\.\)=.+\]$", re.IGNORECASE)
_ROW_CONTEXT_XPATH = re.compile(r"^//(?:tr|li|\*\[@role='row'\]|\*\[@role='listitem'\]|\*\[contains\(concat\(' ', normalize-space\(@class\), ' '\), ' [^']+ '\)\])\[\.//\*\[normalize-space\(\.\)=.+\]\]//[a-z][a-z0-9-]*\[normalize-space\(\.\)=.+\]$", re.IGNORECASE)
_ROW_INDEX_XPATH = re.compile(r"^(?:\((//(?:tr|li|\*\[@role='row'\]|\*\[@role='listitem'\]|\*\[contains\(concat\(' ', normalize-space\(@class\), ' '\), ' [^']+ '\)\]))\)\[[0-9]+\]//[a-z][a-z0-9-]*\[normalize-space\(\.\)=.+\]|\((//(?:tr|li|\*\[@role='row'\]|\*\[@role='listitem'\]|\*\[contains\(concat\(' ', normalize-space\(@class\), ' '\), ' [^']+ '\)\])\[\.//\*\[normalize-space\(\.\)=.+\]\]//[a-z][a-z0-9-]*\[normalize-space\(\.\)=.+\])\)\[[0-9]+\]|\(//[a-z][a-z0-9-]*\[normalize-space\(\.\)=.+\]\)\[[0-9]+\])$", re.IGNORECASE)
_CSS_SENSITIVE_ATTRIBUTE = re.compile(r"\[\s*(?:value|password|passwd|token|access_token|refresh_token|api[-_]?key|apikey|session|cookie|authorization|secret|client_secret)\b", re.IGNORECASE)


@dataclass(frozen=True)
class _LocatorSeed:
    strategy: str
    locator_type: str
    value: str


def is_dynamic_token(value: str) -> bool:
    """按单个 token 判断随机、框架生成或时变标识，避免误杀业务日期。"""
    token = value.strip()
    if not token:
        return False
    if _UUID.fullmatch(token) or _REACT_ID.fullmatch(token) or _CSS_HASH.fullmatch(token) or _CSS_MODULE.fullmatch(token):
        return True
    digits = re.findall(r"\d+", token)
    if any(len(part) in {10, 13} for part in digits):
        return True
    for part in digits:
        if len(part) >= 6 and not (len(part) == 8 and part.startswith(("19", "20"))):
            return True
    if re.fullmatch(r"[a-f0-9]{12,}", token, re.IGNORECASE):
        return True
    return _looks_high_entropy_token(token)


def score_locator(strategy: str, unique: bool, depth: int) -> int:
    """基线评分减去非唯一性和过深 DOM 的惩罚，结果受限于 0..100。"""
    semantic_strategy = strategy in {"testid", "id", "framework", "row-context", "role", "form-label", "label", "attribute", "stable-xpath", "row-index"}
    depth_penalty = 0 if unique and semantic_strategy else max(depth - 5, 0) * 2
    score = _SCORES.get(strategy, 0) - (0 if unique else 50) - depth_penalty
    return max(0, min(100, score))


def build_candidate(snapshot: ElementSnapshot) -> CaptureCandidate:
    """从原始快照提取已脱敏的页面元素候选；无可靠定位器时拒绝生成。"""
    attributes = _normalized_attributes(snapshot.attributes)
    accessible_name = _safe_text(snapshot.accessible_name)
    label = _safe_text(snapshot.label)
    form_label = _safe_text(snapshot.form_label)
    visible_text = _safe_text(snapshot.visible_text)
    capture_url = sanitize_public_url(snapshot.capture_url) if snapshot.capture_url else ""
    seeds, rejected_reasons = _locator_seeds(snapshot, attributes, accessible_name, label, form_label, visible_text)
    locators = _build_locators(seeds, snapshot)
    if not locators:
        raise ValueError("候选项缺少可靠定位器")
    semantic_name = accessible_name or label or form_label or visible_text
    name = semantic_name or "未命名元素"
    fingerprint = _fingerprint(snapshot.tag, attributes, semantic_name)
    return CaptureCandidate(
        name=name,
        fingerprint=fingerprint,
        capture_url=capture_url,
        tag_name=_normalize_text(snapshot.tag).lower(),
        accessible_name=accessible_name,
        locators=locators,
        quality_score=max(locator.score for locator in locators),
        rejected_reasons=sorted(set(rejected_reasons)),
    )


def _locator_seeds(
    snapshot: ElementSnapshot, attributes: dict[str, str], accessible_name: str, label: str, form_label: str, visible_text: str,
) -> tuple[list[_LocatorSeed], list[str]]:
    seeds: list[_LocatorSeed] = []
    rejected: list[str] = []
    testid = attributes.get("data-testid", "")
    if _is_safe_token(testid):
        seeds.append(_LocatorSeed("testid", "testid", testid))
    elif testid:
        rejected.append("已过滤不安全定位器")
    element_id = attributes.get("id", "")
    if _is_safe_token(element_id):
        seeds.append(_LocatorSeed("id", "id", element_id))
    elif element_id:
        rejected.append("已过滤不安全定位器")
    role = attributes.get("role", "") or _native_role(snapshot.tag, attributes)
    role_name = accessible_name or visible_text
    if role_name and re.fullmatch(r"[A-Za-z][A-Za-z0-9-]*", role or ""):
        seeds.append(_LocatorSeed("role", "role", f"{role}[name={json.dumps(role_name, ensure_ascii=False)}]"))
    if label:
        seeds.append(_LocatorSeed("label", "label", label))
    if form_label:
        seeds.append(_LocatorSeed("form-label", "xpath", _form_label_xpath(snapshot.tag, form_label)))
    for key in ("name", "aria-label", "placeholder", "autocomplete"):
        value = attributes.get(key, "")
        if _is_safe_token(value):
            seeds.append(_LocatorSeed("attribute", "css", f"[{_css_escape_identifier(key)}={_css_string(value)}]"))
    for key in sorted(attributes):
        value = attributes[key]
        if key.startswith("data-") and key != "data-testid" and _is_safe_token(value):
            seeds.append(_LocatorSeed("css", "css", f"[{_css_escape_identifier(key)}={_css_string(value)}]"))
    css_selector = snapshot.css_selector.strip()
    if css_selector:
        if _is_safe_css(css_selector):
            seeds.append(_LocatorSeed("css", "css", css_selector))
        else:
            rejected.append("已过滤不安全定位器")
    class_selector = _stable_class_selector(snapshot.tag, attributes.get("class", ""))
    if class_selector:
        seeds.append(_LocatorSeed("css", "css", class_selector))
    for class_name in _stable_business_classes(attributes.get("class", "")):
        seeds.append(_LocatorSeed("framework", "css", f"[class~={_css_string(class_name)}]"))
    if visible_text:
        seeds.append(_LocatorSeed("text", "text", visible_text))
    xpath = snapshot.xpath.strip()
    if xpath:
        stable_xpath = bool(_STABLE_XPATH.fullmatch(xpath))
        row_context_xpath = bool(_ROW_CONTEXT_XPATH.fullmatch(xpath))
        row_index_xpath = bool(_ROW_INDEX_XPATH.fullmatch(xpath))
        if row_context_xpath or row_index_xpath or stable_xpath or _is_safe_xpath(xpath):
            strategy = "row-context" if row_context_xpath else "row-index" if row_index_xpath else "stable-xpath" if stable_xpath else "xpath"
            seeds.append(_LocatorSeed(strategy, "xpath", xpath))
        else:
            rejected.append("已过滤不安全定位器")
    return seeds, rejected


def _build_locators(seeds: list[_LocatorSeed], snapshot: ElementSnapshot) -> list[CaptureLocator]:
    deduplicated: dict[tuple[str, str], _LocatorSeed] = {}
    for seed in seeds:
        deduplicated.setdefault((seed.locator_type, seed.value), seed)
    locators: list[tuple[int, CaptureLocator]] = []
    for seed in deduplicated.values():
        count = _match_count(snapshot, seed.strategy, seed.value)
        unique = count == 1
        locator = CaptureLocator(
            type=seed.locator_type,
            value=seed.value,
            score=score_locator(seed.strategy, unique, snapshot.depth),
            unique=unique,
            match_count=count,
        )
        locators.append((_PRIORITY[seed.strategy], locator))
    locators.sort(key=lambda item: (item[0], -item[1].score, item[1].type, item[1].value))
    selected = locators[:3]
    xpath = next((item for item in locators if item[1].type == "xpath"), None)
    if xpath and xpath not in selected:
        selected[-1:] = [xpath]
    return [locator for _, locator in selected]


def _match_count(snapshot: ElementSnapshot, strategy: str, value: str) -> int | None:
    match_key = {"form-label": "xpath", "stable-xpath": "xpath", "row-context": "xpath", "row-index": "xpath", "framework": "css", "attribute": "css"}.get(strategy, strategy)
    raw = snapshot.locator_matches.get(
        f"{strategy}:{value}",
        snapshot.locator_matches.get(f"{match_key}:{value}", snapshot.locator_matches.get(value)),
    )
    if isinstance(raw, bool) or not isinstance(raw, int) or raw < 0:
        return None
    return raw


def _normalized_attributes(raw_attributes: dict[str, object]) -> dict[str, str]:
    attributes: dict[str, str] = {}
    for raw_key, raw_value in raw_attributes.items():
        key = _normalize_text(str(raw_key)).lower()
        value = _normalize_text(str(raw_value))
        if not key or not value or _is_sensitive_key(key) or _contains_secret(value):
            continue
        attributes[key] = value
    return attributes


def _fingerprint(tag: str, attributes: dict[str, str], name: str) -> str:
    stable_attributes: dict[str, str] = {}
    for key, value in attributes.items():
        if key == "class":
            classes = sorted({_normalize_text(part) for part in value.split() if _is_safe_token(part)})
            if classes:
                stable_attributes[key] = " ".join(classes)
        elif _is_safe_token(value):
            stable_attributes[key] = value
    canonical = {
        "tag": _normalize_text(tag).lower(),
        "attributes": dict(sorted(stable_attributes.items())),
        "name": _normalize_text(name),
    }
    serialized = json.dumps(canonical, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


def _is_safe_css(value: str) -> bool:
    if len(value) > 240 or _CONTROL.search(value) or _contains_secret(value) or _CSS_SENSITIVE_ATTRIBUTE.search(value):
        return False
    return not any(is_dynamic_token(token) for token in re.findall(r"[A-Za-z_][A-Za-z0-9_-]*|:[A-Za-z0-9]+:", value))


def _is_safe_xpath(value: str) -> bool:
    if len(value) > 240 or _CONTROL.search(value) or _contains_secret(value) or _XPATH_SENSITIVE_ATTRIBUTE.search(value):
        return False
    literals = re.findall(r"(['\"])(.*?)\1", value)
    return not any(_contains_secret(literal) or is_dynamic_token(literal) for _, literal in literals)


def _is_sensitive_key(value: str) -> bool:
    return is_sensitive_key(_normalize_text(value))


def _contains_secret(value: str) -> bool:
    return contains_secret_text(_normalize_text(value))


def _is_safe_token(value: str) -> bool:
    return bool(value) and not _CONTROL.search(value) and not _contains_secret(value) and not is_dynamic_token(value)


def _safe_text(value: str) -> str:
    normalized = _normalize_text(value)
    return normalized if normalized and not _contains_secret(normalized) and not _CONTROL.search(normalized) else ""


def _stable_class_selector(tag: str, classes: str) -> str:
    stable_classes = sorted({_normalize_text(token) for token in classes.split() if _is_safe_token(token)})
    if not stable_classes:
        return ""
    return _css_escape_identifier(_normalize_text(tag).lower()) + "".join(f".{_css_escape_identifier(token)}" for token in stable_classes)


def _stable_business_classes(classes: str) -> list[str]:
    tokens = sorted({_normalize_text(token) for token in classes.split() if _is_safe_token(token)})
    return [token for token in tokens if "_" in token]


def _native_role(tag: str, attributes: dict[str, str]) -> str:
    normalized_tag = _normalize_text(tag).lower()
    if normalized_tag == "button":
        return "button"
    if normalized_tag == "a" and attributes.get("href"):
        return "link"
    return ""


def _form_label_xpath(tag: str, label: str) -> str:
    safe_tag = _normalize_text(tag).lower()
    if not re.fullmatch(r"[a-z][a-z0-9-]{0,63}", safe_tag):
        safe_tag = "*"
    form_item = "contains(concat(' ', normalize-space(@class), ' '), ' el-form-item ')"
    return f"//*[self::fieldset or @role='group' or {form_item}][.//*[normalize-space()={_xpath_literal(label)}]]//{safe_tag}"


def _xpath_literal(value: str) -> str:
    if "'" not in value:
        return f"'{value}'"
    if '"' not in value:
        return f'"{value}"'
    return "concat(" + ", \"'\", ".join(f"'{part}'" for part in value.split("'")) + ")"


def _css_string(value: str) -> str:
    escaped = []
    for char in value:
        code = ord(char)
        if char in {'"', "\\"}:
            escaped.append(f"\\{char}")
        elif code < 32 or code == 127:
            escaped.append(f"\\{code:x} ")
        else:
            escaped.append(char)
    return '"' + "".join(escaped) + '"'


def _css_escape_identifier(value: str) -> str:
    escaped = []
    for index, char in enumerate(value):
        code = ord(char)
        if code == 0:
            escaped.append("\ufffd")
        elif code < 32 or code == 127 or (index == 0 and char.isdigit()) or (index == 1 and value[0] == "-" and char.isdigit()):
            escaped.append(f"\\{code:x} ")
        elif char.isalnum() or char in {"-", "_"} or code >= 128:
            escaped.append(char)
        else:
            escaped.append(f"\\{char}")
    return "".join(escaped)


def _normalize_text(value: str) -> str:
    return " ".join(unicodedata.normalize("NFC", value).split())


def _looks_high_entropy_token(value: str) -> bool:
    if len(value) < 24 or not re.fullmatch(r"[A-Za-z0-9_+/=-]+", value):
        return False
    entropy = -sum((value.count(char) / len(value)) * math.log2(value.count(char) / len(value)) for char in set(value))
    return entropy >= 3.5
