from pydantic import BaseModel, Field


class ElementSnapshot(BaseModel):
    """由浏览器层提供的、已脱离 Playwright 的元素特征。"""

    tag: str
    attributes: dict[str, str] = Field(default_factory=dict)
    accessible_name: str = ""
    label: str = ""
    visible_text: str = ""
    css_selector: str = ""
    xpath: str = ""
    depth: int = 0
    locator_matches: dict[str, int] = Field(default_factory=dict)
    capture_url: str = ""


class CaptureLocator(BaseModel):
    type: str
    value: str
    score: int
    unique: bool
    match_count: int = Field(alias="matchCount")

    model_config = {"populate_by_name": True}

    def platform_payload(self) -> dict[str, str | int | bool]:
        return {
            "type": self.type,
            "value": self.value,
            "score": self.score,
            "unique": self.unique,
        }


class CaptureCandidate(BaseModel):
    name: str
    fingerprint: str
    capture_url: str = Field(default="", alias="captureUrl")
    tag_name: str = Field(alias="tagName")
    accessible_name: str = Field(alias="accessibleName")
    locators: list[CaptureLocator]
    quality_score: int = Field(alias="qualityScore")
    rejected_reasons: list[str] = Field(default_factory=list, alias="rejectedReasons")

    model_config = {"populate_by_name": True}

    def platform_payload(self) -> dict[str, object]:
        """返回可直接传给平台候选接收接口的 JSON 字段。"""
        return {
            "name": self.name,
            "fingerprint": self.fingerprint,
            "captureUrl": self.capture_url,
            "tagName": self.tag_name,
            "accessibleName": self.accessible_name,
            "locators": [locator.platform_payload() for locator in self.locators],
            "qualityScore": self.quality_score,
        }
