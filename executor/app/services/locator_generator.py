import hashlib
import json
import re

from app.models.capture import CaptureCandidate, CaptureLocator, ElementSnapshot


_DYNAMIC_TOKEN = re.compile(
    r"(?:"
    r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
    r"|\b\d{10,13}\b"
    r"|(?<!\d)\d{6,}(?!\d)"
    r"|\b[a-f0-9]{12,}\b"
    r"|^(?:css|sc|emotion|jss|mui)-[a-z0-9_-]{5,}$"
    r")",
    re.IGNORECASE,
)
_SENSITIVE_WORD = re.compile(r"password|token|secret|cookie|authorization|bearer", re.IGNORECASE)
_SENSITIVE_ATTRIBUTE = re.compile(r"password|token|secret|cookie|authorization|value", re.IGNORECASE)
_SCORES = {"testid": 95, "id": 90, "role": 85, "label": 82, "css": 70, "text": 60, "xpath": 40}


def is_dynamic_token(value: str) -> bool:
    """判断属性值是否明显包含随机或时变片段。"""
    return bool(_DYNAMIC_TOKEN.search(value.strip()))


def score_locator(strategy: str, unique: bool, depth: int) -> int:
    """按约定基线、唯一性和 DOM 深度计算 0..100 定位器质量分。"""
    score = _SCORES.get(strategy, 0)
    if not unique:
        score -= 50
    score -= max(depth - 5, 0) * 2
    return max(0, min(100, score))


def build_candidate(snapshot: ElementSnapshot) -> CaptureCandidate:
    """从脱敏后的 DOM 快照生成至多三组稳定定位器和候选元数据。"""
    attributes = {str(key).lower(): str(value).strip() for key, value in snapshot.attributes.items() if str(value).strip()}
    has_sensitive_content = _snapshot_contains_sensitive_content(attributes, snapshot)
    rejected_reasons: list[str] = []
    candidates: list[tuple[str, str, str]] = []

    testid = attributes.get("data-testid", "")
    if _is_safe_token(testid):
        candidates.append(("testid", "testid", testid))
    elif testid:
        rejected_reasons.append("已过滤动态或敏感 data-testid")

    for key in sorted(attributes):
        if not key.startswith("data-") or key == "data-testid":
            continue
        value = attributes[key]
        if _is_sensitive_attribute(key) or not _is_safe_token(value):
            rejected_reasons.append(f"已过滤不安全属性 {key}")
            continue
        candidates.append(("css", "css", f'[{key}="{value}"]'))

    element_id = attributes.get("id", "")
    if _is_safe_token(element_id):
        candidates.append(("id", "id", element_id))
    elif element_id:
        rejected_reasons.append("已过滤动态或敏感 id")

    accessible_name = "" if has_sensitive_content else _safe_text(snapshot.accessible_name)
    label = "" if has_sensitive_content else _safe_text(snapshot.label)
    visible_text = "" if has_sensitive_content else _safe_text(snapshot.visible_text)
    role = attributes.get("role", "")
    if role and accessible_name and _is_safe_token(role):
        candidates.append(("role", "role", f'{role}[name="{accessible_name}"]'))
    if label:
        candidates.append(("label", "label", label))

    css_selector = snapshot.css_selector.strip()
    if css_selector and _is_safe_expression(css_selector):
        candidates.append(("css", "css", css_selector))
    else:
        class_selector = _stable_class_selector(snapshot.tag, attributes.get("class", ""))
        if class_selector:
            candidates.append(("css", "css", class_selector))
        elif css_selector:
            rejected_reasons.append("已过滤动态或敏感 CSS")

    if visible_text:
        candidates.append(("text", "text", visible_text))
    xpath = snapshot.xpath.strip()
    if xpath and _is_safe_expression(xpath):
        candidates.append(("xpath", "xpath", xpath))
    elif xpath:
        rejected_reasons.append("已过滤动态或敏感 XPath")

    locators = _build_locators(candidates, snapshot)
    fingerprint = _fingerprint(snapshot, attributes, accessible_name)
    name = accessible_name or label or visible_text or "未命名元素"
    return CaptureCandidate(
        name=name,
        fingerprint=fingerprint,
        capture_url=snapshot.capture_url,
        tag_name=snapshot.tag.lower().strip(),
        accessible_name=accessible_name,
        locators=locators,
        quality_score=max((locator.score for locator in locators), default=0),
        rejected_reasons=sorted(set(rejected_reasons)),
    )


def _build_locators(candidates: list[tuple[str, str, str]], snapshot: ElementSnapshot) -> list[CaptureLocator]:
    locators: list[CaptureLocator] = []
    seen: set[tuple[str, str]] = set()
    for strategy, locator_type, value in candidates:
        key = (locator_type, value)
        if key in seen:
            continue
        seen.add(key)
        count = _match_count(snapshot, strategy, value)
        locators.append(
            CaptureLocator(
                type=locator_type,
                value=value,
                score=score_locator(strategy, unique=count == 1, depth=snapshot.depth),
                unique=count == 1,
                match_count=count,
            )
        )
        if len(locators) == 3:
            break
    return locators


def _match_count(snapshot: ElementSnapshot, strategy: str, value: str) -> int:
    value_key = f"{strategy}:{value}"
    raw = snapshot.locator_matches.get(value_key, snapshot.locator_matches.get(value, 1))
    try:
        return max(0, int(raw))
    except (TypeError, ValueError):
        return 1


def _fingerprint(snapshot: ElementSnapshot, attributes: dict[str, str], accessible_name: str) -> str:
    stable_attributes = {
        key: value
        for key, value in attributes.items()
        if not _is_sensitive_attribute(key) and _is_safe_token(value)
    }
    payload = {
        "tag": snapshot.tag.lower().strip(),
        "attributes": stable_attributes,
        "accessibleName": accessible_name,
    }
    normalized = json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(normalized.encode("utf-8")).hexdigest()


def _snapshot_contains_sensitive_content(attributes: dict[str, str], snapshot: ElementSnapshot) -> bool:
    if any(_is_sensitive_attribute(key) or _SENSITIVE_WORD.search(value) for key, value in attributes.items()):
        return True
    return any(_SENSITIVE_WORD.search(value) for value in (snapshot.accessible_name, snapshot.label, snapshot.visible_text))


def _is_safe_token(value: str) -> bool:
    return bool(value) and not _SENSITIVE_WORD.search(value) and not is_dynamic_token(value)


def _is_safe_expression(value: str) -> bool:
    if not _is_safe_token(value) or len(value) > 240:
        return False
    return not any(is_dynamic_token(token) for token in re.findall(r"[A-Za-z0-9_-]+", value))


def _is_sensitive_attribute(key: str) -> bool:
    return bool(_SENSITIVE_ATTRIBUTE.search(key))


def _safe_text(value: str) -> str:
    return value.strip() if value.strip() and not _SENSITIVE_WORD.search(value) else ""


def _stable_class_selector(tag: str, classes: str) -> str:
    stable_classes = [token for token in classes.split() if _is_safe_token(token)]
    if not stable_classes:
        return ""
    return f"{tag.lower().strip()}" + "".join(f".{token}" for token in sorted(set(stable_classes)))
