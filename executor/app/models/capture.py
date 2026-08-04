from enum import Enum
from typing import Any, Literal

from pydantic import BaseModel, Field


class CaptureMode(str, Enum):
    pick = "pick"
    operate = "operate"


class CaptureCommand(BaseModel):
    """平台租约命令；receipt 只驻留内存，绝不写入日志或本地文件。"""

    id: int = Field(gt=0)
    session_id: str = Field(alias="sessionId", min_length=1)
    type: Literal["start", "set_mode", "stop", "expire"]
    mode: CaptureMode | None = None
    url: str = ""
    browser_channel: str = Field(default="", alias="browserChannel")
    token: str = Field(default="", repr=False)
    receipt: str = Field(default="", repr=False)
    headless: bool = False

    model_config = {"populate_by_name": True}


class CaptureStartCommand(CaptureCommand):
    type: Literal["start"] = "start"
    mode: CaptureMode = CaptureMode.pick
    url: str = Field(min_length=1)
    browser_channel: str = Field(alias="browserChannel", min_length=1)
    token: str = Field(min_length=1, repr=False)
    receipt: str = Field(min_length=1, repr=False)


class CaptureState(BaseModel):
    """可对本地 health/GUI 暴露的非敏感会话状态。"""

    session_id: str = Field(alias="sessionId")
    browser_context_id: str = Field(alias="browserContextId")
    mode: CaptureMode
    page_title: str = Field(default="", alias="pageTitle")
    active: bool = True

    model_config = {"populate_by_name": True}


class ElementSnapshot(BaseModel):
    """浏览器层输入的原始元素特征；原始字段绝不参与模型序列化。"""

    tag: str = Field(min_length=1)
    attributes: dict[str, Any] = Field(default_factory=dict, exclude=True, repr=False)
    accessible_name: str = Field(default="", exclude=True, repr=False)
    label: str = Field(default="", exclude=True, repr=False)
    form_label: str = Field(default="", exclude=True, repr=False)
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
    try:
        from app.services.capture_security import sanitize_public_url

        return bool(value) and sanitize_public_url(value) == value
    except ValueError:
        return False
