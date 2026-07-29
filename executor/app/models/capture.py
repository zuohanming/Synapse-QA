from typing import Any

from pydantic import BaseModel, Field


class ElementSnapshot(BaseModel):
    """浏览器层输入的原始元素特征；原始字段绝不参与模型序列化。"""

    tag: str = Field(min_length=1)
    attributes: dict[str, Any] = Field(default_factory=dict, exclude=True, repr=False)
    accessible_name: str = Field(default="", exclude=True, repr=False)
    label: str = Field(default="", exclude=True, repr=False)
    visible_text: str = Field(default="", exclude=True, repr=False)
    css_selector: str = Field(default="", exclude=True, repr=False)
    xpath: str = Field(default="", exclude=True, repr=False)
    depth: int = Field(default=0, ge=0)
    locator_matches: dict[str, Any] = Field(default_factory=dict, exclude=True, repr=False)
    capture_url: str = Field(default="", exclude=True, repr=False)


class CaptureLocator(BaseModel):
    type: str = Field(min_length=1)
    value: str = Field(min_length=1)
    score: int = Field(ge=0, le=100)
    unique: bool
    match_count: int | None = Field(default=None, alias="matchCount", ge=0)

    model_config = {"populate_by_name": True}

    def platform_payload(self) -> dict[str, str | int | bool]:
        return {"type": self.type, "value": self.value, "score": self.score, "unique": self.unique}


class CaptureCandidate(BaseModel):
    name: str = Field(min_length=1)
    fingerprint: str = Field(pattern=r"^[a-f0-9]{64}$")
    capture_url: str = Field(default="", alias="captureUrl")
    tag_name: str = Field(alias="tagName", min_length=1)
    accessible_name: str = Field(alias="accessibleName")
    locators: list[CaptureLocator] = Field(min_length=1, max_length=3)
    quality_score: int = Field(alias="qualityScore", ge=0, le=100)
    rejected_reasons: list[str] = Field(default_factory=list, alias="rejectedReasons")

    model_config = {"populate_by_name": True}

    def platform_payload(self) -> dict[str, object]:
        if not _is_sanitized_http_url(self.capture_url):
            raise ValueError("采集 URL 必须是已脱敏的 HTTP(S) 地址")
        return {
            "name": self.name,
            "fingerprint": self.fingerprint,
            "captureUrl": self.capture_url,
            "tagName": self.tag_name,
            "accessibleName": self.accessible_name,
            "locators": [locator.platform_payload() for locator in self.locators],
            "qualityScore": self.quality_score,
        }


def _is_sanitized_http_url(value: str) -> bool:
    from urllib.parse import parse_qsl, urlsplit

    if not value or any(ord(char) < 32 or ord(char) == 127 for char in value):
        return False
    try:
        parsed = urlsplit(value)
        port = parsed.port
    except ValueError:
        return False
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password or parsed.fragment:
        return False
    if port is not None and not 0 < port < 65536:
        return False
    sensitive_parts = {"password", "passwd", "token", "access_token", "refresh_token", "apikey", "api_key", "session", "cookie", "authorization", "secret", "client_secret"}
    for key, _ in parse_qsl(parsed.query, keep_blank_values=True):
        normalized = key.lower().replace("-", "_").replace(".", "_")
        parts = [part for part in normalized.replace("/", "_").replace(":", "_").split("_") if part]
        if normalized in sensitive_parts or any(part in sensitive_parts for part in parts):
            return False
    return True
