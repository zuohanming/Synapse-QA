import pytest

from app.models.capture import CaptureCandidate, CaptureLocator, ElementSnapshot
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


def test_generates_unique_form_context_locator_for_inputs_sharing_component_class():
    xpath = "//*[self::fieldset or @role='group' or contains(concat(' ', normalize-space(@class), ' '), ' el-form-item ')][.//*[normalize-space()='账号']]//input"
    candidate = build_candidate(
        ElementSnapshot(
            tag="input",
            attributes={"class": "el-input__inner", "placeholder": "请输入账号"},
            form_label="账号",
            locator_matches={f"xpath:{xpath}": 1, 'css:[placeholder="请输入账号"]': 1, "css:input.el-input__inner": 2},
        )
    )

    form_locator = next(locator for locator in candidate.locators if locator.value == xpath)
    assert form_locator.type == "xpath"
    assert form_locator.unique
    assert form_locator.score >= 70
    assert candidate.name == "账号"


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
            visible_text="安全按钮",
            css_selector="button.css-1a2b3c",
            locator_matches={"text:安全按钮": 1},
        )
    )

    assert all("css-1a2b3c" not in locator.value for locator in candidate.locators)


def test_excludes_sensitive_values_from_candidate_locator_and_fingerprint():
    candidate = build_candidate(
        ElementSnapshot(
            tag="input",
            attributes={"type": "password", "value": "never-store-me", "data-token": "top-secret", "id": "password-field"},
            accessible_name="密码",
            visible_text="top-secret",
            locator_matches={"id:password-field": 1},
        )
    )

    payload = candidate.model_dump(by_alias=True, mode="json")
    assert "never-store-me" not in str(payload)
    assert "top-secret" not in str(payload)
    assert candidate.name == "密码"


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
    candidate = build_candidate(ElementSnapshot(tag="div", attributes={"id": "notice"}, visible_text="secret"))

    assert candidate.name == "未命名元素"


def test_snapshot_hides_raw_sensitive_data_from_repr_str_and_serialization():
    snapshot = ElementSnapshot(
        tag="input",
        attributes={"access_token": "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature"},
        visible_text="sk-live-super-secret",
        xpath="//*[@value='sk-live-super-secret']",
        capture_url="https://user:password@example.test/path?access_token=top-secret",
    )

    for representation in (repr(snapshot), str(snapshot), str(snapshot.model_dump(mode="json"))):
        assert "top-secret" not in representation
        assert "sk-live-super-secret" not in representation
        assert "eyJhbGci" not in representation


def test_sanitizes_capture_url_before_exposing_platform_payload():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"id": "save"},
            accessible_name="保存",
            capture_url="https://alice:password@example.test:8443/orders?tab=details&access_token=top-secret#private",
            locator_matches={"id:save": 1},
        )
    )

    payload = candidate.platform_payload()
    assert payload["captureUrl"] == "https://example.test:8443/orders"
    assert "password" not in str(payload)
    assert "top-secret" not in str(payload)


@pytest.mark.parametrize("url", ["javascript:alert(1)", "ftp://example.test/file", "https://example.test/%00"])
def test_rejects_non_http_or_control_character_capture_url(url):
    with pytest.raises(ValueError):
        build_candidate(ElementSnapshot(tag="button", attributes={"id": "save"}, capture_url=url))


def test_preserves_safe_password_accessible_name_without_sensitive_value():
    candidate = build_candidate(
        ElementSnapshot(
            tag="input",
            attributes={"type": "password", "id": "account-password"},
            accessible_name="密码",
            label="登录密码",
            locator_matches={"id:account-password": 1},
        )
    )

    assert candidate.name == "密码"
    assert candidate.accessible_name == "密码"


def test_sensitive_key_matching_uses_exact_tokens_without_false_positives():
    candidate = build_candidate(
        ElementSnapshot(
            tag="input",
            attributes={"secretary": "assistant", "aria-valuemax": "100", "id": "profile"},
            accessible_name="秘书",
            locator_matches={"id:profile": 1},
        )
    )

    assert candidate.name == "秘书"
    assert candidate.fingerprint != build_candidate(
        ElementSnapshot(tag="input", attributes={"id": "profile"}, accessible_name="秘书", locator_matches={"id:profile": 1})
    ).fingerprint


@pytest.mark.parametrize(
    "value",
    [
        "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature",
        "Bearer sk-live-4f052e637b014b2c91e29a58",
        "AKIAIOSFODNN7EXAMPLE",
        "ghp_abcdefghijklmnopqrstuvwxyz1234567890",
    ],
)
def test_filters_jwt_bearer_and_common_secret_prefixes(value):
    candidate = build_candidate(
        ElementSnapshot(tag="input", attributes={"data-value": value, "id": "safe"}, locator_matches={"id:safe": 1})
    )

    assert value not in str(candidate.model_dump(by_alias=True, mode="json"))


def test_unknown_zero_and_invalid_match_counts_can_never_be_unique_or_reliable():
    unknown = build_candidate(ElementSnapshot(tag="button", attributes={"id": "unknown"}))
    zero = build_candidate(ElementSnapshot(tag="button", attributes={"id": "zero"}, locator_matches={"id:zero": 0}))
    invalid = build_candidate(ElementSnapshot(tag="button", attributes={"id": "invalid"}, locator_matches={"id:invalid": "many"}))

    for candidate in (unknown, zero, invalid):
        locator = candidate.locators[0]
        assert not locator.unique
        assert locator.score <= 45
    assert unknown.locators[0].match_count is None
    assert zero.locators[0].match_count == 0
    assert invalid.locators[0].match_count is None


def test_orders_complete_locator_set_by_strategy_before_truncating_to_three():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"data-qa": "save", "id": "save", "role": "button", "class": "button"},
            accessible_name="保存",
            label="提交订单",
            visible_text="保存",
            css_selector="button.button",
            xpath="//button[@id='save']",
            locator_matches={"id:save": 1, 'role:button[name="保存"]': 1, "label:提交订单": 1},
        )
    )

    assert [locator.type for locator in candidate.locators] == ["id", "role", "label"]


def test_text_and_xpath_follow_css_when_higher_priority_locators_are_absent():
    candidate = build_candidate(
        ElementSnapshot(
            tag="div",
            visible_text="操作成功",
            xpath="//div[@data-state='done']",
            locator_matches={"text:操作成功": 1, "xpath://div[@data-state='done']": 1},
        )
    )

    assert [locator.type for locator in candidate.locators] == ["text", "xpath"]


def test_escapes_generated_css_and_role_name_literals():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"data-qa": 'save"\\now', "role": "button"},
            accessible_name='保存"订单',
            locator_matches={'role:button[name="保存\\"订单"]': 1},
        )
    )

    css = next(locator.value for locator in candidate.locators if locator.type == "css")
    role = next(locator.value for locator in candidate.locators if locator.type == "role")
    assert css == '[data-qa="save\\"\\\\now"]'
    assert role == 'button[name="保存\\"订单"]'


@pytest.mark.parametrize("selector", ["//input[@value='secret']", "//*[@name='access_token']", "//div[\u0001]"])
def test_rejects_sensitive_or_unsafe_external_xpath(selector):
    candidate = build_candidate(
        ElementSnapshot(tag="div", visible_text="安全文本", xpath=selector, locator_matches={"text:安全文本": 1})
    )

    assert all(locator.type != "xpath" for locator in candidate.locators)


@pytest.mark.parametrize("token", ["550e8400-e29b-41d4-a716-446655440000", ":r0:", "Button_module__a1B2c", "css-1a2b3c"])
def test_detects_dynamic_tokens_but_keeps_business_date_identifier(token):
    assert is_dynamic_token(token)
    assert not is_dynamic_token("order-20260729")


def test_fingerprint_normalizes_unicode_whitespace_and_class_order():
    first = build_candidate(
        ElementSnapshot(
            tag="BUTTON",
            attributes={"class": "primary action", "id": "save"},
            accessible_name=" 保存 ",
            locator_matches={"id:save": 1},
        )
    )
    second = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"id": "save", "class": "action primary css-1a2b3c"},
            accessible_name="保存",
            locator_matches={"id:save": 1},
        )
    )

    assert first.fingerprint == second.fingerprint


def test_rejects_candidate_without_a_safe_locator_and_platform_contract_rejects_bad_direct_model():
    with pytest.raises(ValueError, match="可靠定位器"):
        build_candidate(ElementSnapshot(tag="div", attributes={"value": "top-secret"}))
    with pytest.raises(ValueError):
        CaptureCandidate(
            name="保存",
            fingerprint="a" * 64,
            captureUrl="https://example.test",
            tagName="button",
            accessibleName="保存",
            locators=[CaptureLocator(type="id", value="save", score=101, unique=True, matchCount=1)],
            qualityScore=101,
        )


def test_platform_payload_matches_go_candidate_gate_and_rejects_unsanitized_direct_url():
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"id": "save"},
            accessible_name="保存",
            capture_url="https://example.test/orders",
            locator_matches={"id:save": 1},
        )
    )

    payload = candidate.platform_payload()
    assert set(payload) == {"name", "fingerprint", "captureUrl", "tagName", "accessibleName", "locators", "qualityScore"}
    assert 1 <= len(payload["locators"]) <= 3
    assert set(payload["locators"][0]) == {"type", "value", "score", "unique"}
    with pytest.raises(ValueError):
        CaptureCandidate(
            name="保存",
            fingerprint="a" * 64,
            captureUrl="https://example.test/?data-token=secret",
            tagName="button",
            accessibleName="保存",
            locators=[CaptureLocator(type="id", value="save", score=90, unique=True, matchCount=1)],
            qualityScore=90,
        ).platform_payload()


def test_escapes_css_identifier_for_digit_and_special_character_class_tokens():
    candidate = build_candidate(
        ElementSnapshot(
            tag="div",
            attributes={"class": "9item menu:item"},
            visible_text="菜单",
            locator_matches={"css:div.\\39 item.menu\\:item": 1},
        )
    )

    assert any(locator.value == "div.\\39 item.menu\\:item" for locator in candidate.locators)
