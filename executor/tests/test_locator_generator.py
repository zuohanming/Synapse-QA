from app.models.capture import ElementSnapshot
from app.services.locator_generator import build_candidate, is_dynamic_token, score_locator


def test_prefers_testid_over_static_id_and_css():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"data-testid": "save-order", "id": "save-order-button", "class": "primary"},
            accessible_name="保存",
            visible_text="保存",
            css_selector="button.primary",
        )
    )

    assert candidate.locators[0].type == "testid"
    assert candidate.locators[0].value == "save-order"


def test_generates_priority_order_for_role_label_css_text_and_xpath():
    candidate = build_candidate(
        ElementSnapshot(
            tag="input",
            attributes={"role": "textbox", "class": "form-control", "placeholder": "邮箱"},
            accessible_name="邮箱",
            label="电子邮箱",
            visible_text="邮箱",
            css_selector="form .email-input",
            xpath="//form//input[@name='email']",
        )
    )

    assert [locator.type for locator in candidate.locators] == ["role", "label", "css"]


def test_filters_dynamic_id_and_class_but_keeps_stable_business_id():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"id": "btn_981273", "class": "css-1a2b3c primary", "data-testid": "save-order"},
            accessible_name="保存",
        )
    )
    stable = build_candidate(ElementSnapshot(tag="button", attributes={"id": "save-order"}, accessible_name="保存"))

    assert all("981273" not in locator.value and "css-1a2b3c" not in locator.value for locator in candidate.locators)
    assert any(locator.type == "id" and locator.value == "save-order" for locator in stable.locators)
    assert is_dynamic_token("btn_981273")
    assert not is_dynamic_token("save-order")


def test_filters_dynamic_class_embedded_in_css_expression():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"class": "css-1a2b3c"},
            accessible_name="保存",
            css_selector="button.css-1a2b3c",
        )
    )

    assert all("css-1a2b3c" not in locator.value for locator in candidate.locators)


def test_excludes_sensitive_values_from_candidate_locator_and_fingerprint():
    candidate = build_candidate(
        ElementSnapshot(
            tag="input",
            attributes={"type": "password", "value": "never-store-me", "data-token": "top-secret"},
            accessible_name="密码",
            visible_text="top-secret",
        )
    )

    payload = candidate.model_dump(by_alias=True, mode="json")
    assert "never-store-me" not in str(payload)
    assert "top-secret" not in str(payload)
    assert candidate.name == "未命名元素"


def test_limits_deduplicates_and_stably_orders_locators():
    snapshot = ElementSnapshot(
        tag="button",
        attributes={"data-testid": "save", "id": "save", "class": "primary primary"},
        accessible_name="保存",
        visible_text="保存",
        css_selector="button.primary",
        xpath="//button[normalize-space()='保存']",
    )

    first = build_candidate(snapshot)
    second = build_candidate(snapshot)

    assert len(first.locators) == 3
    assert [(item.type, item.value) for item in first.locators] == [(item.type, item.value) for item in second.locators]
    assert len({(item.type, item.value) for item in first.locators}) == len(first.locators)


def test_non_unique_and_deep_locators_are_scored_lower():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"data-testid": "save"},
            accessible_name="保存",
            depth=12,
            locator_matches={"testid:save": 2},
        )
    )

    locator = candidate.locators[0]
    assert not locator.unique
    assert locator.match_count == 2
    assert locator.score == score_locator("testid", unique=False, depth=12)
    assert locator.score < 45


def test_fingerprint_is_stable_across_attribute_order_and_sensitive_changes():
    first = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"id": "save-order", "data-testid": "save", "value": "secret-a"},
            accessible_name="保存",
        )
    )
    second = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"data-testid": "save", "value": "secret-b", "id": "save-order"},
            accessible_name="保存",
        )
    )

    assert first.fingerprint == second.fingerprint
    assert len(first.fingerprint) == 64
    assert first.fingerprint == first.fingerprint.lower()


def test_uses_unnamed_placeholder_when_no_safe_semantic_name_exists():
    candidate = build_candidate(ElementSnapshot(tag="div", attributes={"value": "secret"}, visible_text="secret"))

    assert candidate.name == "未命名元素"
