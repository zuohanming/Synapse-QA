import pytest

from app.models.capture import ElementSnapshot
from app.services.capture_security import (
    CaptureURLSecurityError,
    contains_sensitive_data,
    is_same_capture_site,
    sanitize_public_url,
    validate_network_target,
)
from app.services.locator_generator import build_candidate


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        (
            "https://alice:password@example.test:8443/orders?tab=details&q=sk-live-secret#private",
            "https://example.test:8443/orders",
        ),
        ("https://example.test/", "https://example.test/"),
    ],
)
def test_public_url_keeps_only_origin_and_path(raw, expected):
    assert sanitize_public_url(raw) == expected


@pytest.mark.parametrize(
    "url",
    [
        "https://example.test/files/eyJhbGciOiJIUzI1NiJ9.payload123.signature123",
        "https://example.test/files/%65%79%4A%68%62%47%63%69%4F%69%4A%49%55%7A%49%31%4E%69%4A%39.payload123.signature123",
        "https://example.test/files/sk-live-1234567890abcdef",
    ],
)
def test_public_url_rejects_embedded_secret_in_decoded_path(url):
    with pytest.raises(CaptureURLSecurityError):
        sanitize_public_url(url)


@pytest.mark.parametrize(
    "value",
    [
        {"q": "prefix sk-live-1234567890abcdef suffix"},
        {"path": "/files/eyJhbGciOiJIUzI1NiJ9.payload123.signature123/download"},
        {"passwordValue": "ordinary"},
        {"clientSecret": "ordinary"},
        {"nested": [{"accessToken": "ordinary"}]},
    ],
)
def test_shared_sensitive_scanner_catches_review_counterexamples(value):
    assert contains_sensitive_data(value)


def test_shared_sensitive_scanner_keeps_non_sensitive_counterexamples():
    assert not contains_sensitive_data({"secretary": "assistant", "aria-valuemax": "100", "type": "password"})


@pytest.mark.parametrize("host", ["127.0.0.1", "169.254.169.254", "localhost"])
def test_network_target_rejects_private_loopback_and_metadata_by_default(host):
    with pytest.raises(CaptureURLSecurityError):
        validate_network_target(f"http://{host}/resource")


def test_network_target_allows_explicit_private_host():
    assert validate_network_target(
        "http://127.0.0.1:8090/resource",
        allowed_private_hosts={"127.0.0.1"},
    ) == "http://127.0.0.1:8090"


def test_capture_site_scope_allows_only_sibling_subdomains_of_the_page_site():
    page = "https://signinunifly-qa1.oojoyoo.com/#/login"

    assert is_same_capture_site("https://dragon-gateway-qa1.oojoyoo.com/sso/doLogin", page)
    assert is_same_capture_site("https://signinunifly-qa1.oojoyoo.com/assets/app.js", page)
    assert not is_same_capture_site("https://metadata.oojoyoo.net/latest", page)
    assert not is_same_capture_site("http://127.0.0.1:8090/admin", page)


def test_network_target_resolves_on_every_request_and_rejects_dns_rebinding():
    answers = iter(["93.184.216.34", "127.0.0.1"])

    def resolver(_host, port, **_kwargs):
        return [(2, 1, 6, "", (next(answers), port))]

    assert validate_network_target("https://public.example/resource", resolver=resolver) == "https://public.example"
    with pytest.raises(CaptureURLSecurityError):
        validate_network_target("https://public.example/resource", resolver=resolver)


def test_network_target_enforces_explicit_origin_allowlist():
    with pytest.raises(CaptureURLSecurityError, match="origin"):
        validate_network_target(
            "https://other.example/resource",
            allowed_origins={"https://allowed.example"},
            resolver=lambda *_args, **_kwargs: [(2, 1, 6, "", ("93.184.216.34", 443))],
        )


@pytest.mark.parametrize(
    "attribute_value",
    [
        "prefix sk-live-1234567890abcdef suffix",
        "prefix.eyJhbGciOiJIUzI1NiJ9.payload123.signature123.suffix",
    ],
)
def test_task5_uses_shared_search_based_secret_boundary(attribute_value):
    candidate = build_candidate(
        ElementSnapshot(
            tag="button",
            attributes={"id": "save-order", "data-qa": attribute_value},
            accessible_name="保存",
            locator_matches={"id:save-order": 1},
            capture_url="https://example.test/orders?q=sk-live-1234567890abcdef",
        )
    )

    serialized = str(candidate.platform_payload())
    assert attribute_value not in serialized
    assert candidate.capture_url == "https://example.test/orders"
