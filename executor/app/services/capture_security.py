import ipaddress
import math
import re
import socket
import unicodedata
from typing import Any, Callable, Iterable
from urllib.parse import unquote, urlsplit, urlunsplit


class CaptureURLSecurityError(ValueError):
    pass


_CONTROL = re.compile(r"[\x00-\x1f\x7f]")
_CAMEL_BOUNDARY = re.compile(r"(?<=[a-z0-9])(?=[A-Z])")
_KEY_SEPARATOR = re.compile(r"[^a-z0-9]+")
_SENSITIVE_KEY_PARTS = {
    "password",
    "passwd",
    "token",
    "accesstoken",
    "refreshtoken",
    "apikey",
    "session",
    "cookie",
    "authorization",
    "secret",
    "clientsecret",
}
_SECRET_MARKER = re.compile(
    r"(?<![a-z0-9])(?:access[_ -]?token|refresh[_ -]?token|api[_ -]?key|client[_ -]?secret|"
    r"token|secret|cookie|authorization|bearer)(?![a-z0-9])",
    re.IGNORECASE,
)
_JWT = re.compile(r"eyJ[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{6,}\.[a-zA-Z0-9_-]{6,}")
_COMMON_SECRET = re.compile(
    r"(?<![A-Za-z0-9])(?:(?:sk|pk|ghp|github_pat|xox[baprs]|AIza)[_-][A-Za-z0-9_-]{12,}|"
    r"AKIA[A-Z0-9]{12,})(?![A-Za-z0-9])",
    re.IGNORECASE,
)
_TOKENISH = re.compile(r"[A-Za-z0-9_+/=-]{24,}")


def normalize_key(value: str) -> str:
    expanded = _CAMEL_BOUNDARY.sub("_", str(value).strip())
    return "_".join(part for part in _KEY_SEPARATOR.split(expanded.lower()) if part)


def is_sensitive_key(value: str) -> bool:
    normalized = normalize_key(value)
    if not normalized:
        return False
    compact = normalized.replace("_", "")
    if compact in _SENSITIVE_KEY_PARTS:
        return True
    parts = normalized.split("_")
    return any(part in _SENSITIVE_KEY_PARTS for part in parts)


def contains_secret_text(value: str) -> bool:
    text = " ".join(unicodedata.normalize("NFC", str(value)).split())
    if not text:
        return False
    if _SECRET_MARKER.search(text) or _JWT.search(text) or _COMMON_SECRET.search(text):
        return True
    return any(_looks_high_entropy_token(match.group(0)) for match in _TOKENISH.finditer(text))


def contains_sensitive_data(value: Any, key: str = "") -> bool:
    normalized_key = normalize_key(key)
    if is_sensitive_key(key):
        if not (normalized_key == "type" and isinstance(value, str) and value.lower() in {"password", "tel"}):
            return True
    if isinstance(value, dict):
        locator_payload = (
            "type" in value
            and "value" in value
            and set(value).issubset({"type", "value", "score", "unique", "matchCount"})
        )
        return any(
            contains_sensitive_data(item, "locator_value" if locator_payload and item_key == "value" else str(item_key))
            for item_key, item in value.items()
        )
    if isinstance(value, (list, tuple)):
        return any(contains_sensitive_data(item, key) for item in value)
    if not isinstance(value, str):
        return False
    if normalized_key == "fingerprint" and re.fullmatch(r"[a-f0-9]{64}", value):
        return False
    return contains_secret_text(value)


def sanitize_public_url(value: str) -> str:
    """返回仅含 origin 与 path 的外发 URL；query、fragment、userinfo 一律丢弃。"""

    parsed = _parse_http_url(value)
    decoded_path = parsed.path or "/"
    for _ in range(3):
        next_path = unquote(decoded_path)
        if next_path == decoded_path:
            break
        decoded_path = next_path
    if _CONTROL.search(decoded_path) or contains_secret_text(decoded_path):
        raise CaptureURLSecurityError("采集 URL 路径包含敏感数据")
    return urlunsplit((parsed.scheme.lower(), _safe_netloc(parsed), parsed.path or "/", "", ""))


def url_origin(value: str) -> str:
    parsed = _parse_http_url(value)
    return urlunsplit((parsed.scheme.lower(), _safe_netloc(parsed), "", "", ""))


def same_origin(left: str, right: str) -> bool:
    try:
        return url_origin(left) == url_origin(right)
    except CaptureURLSecurityError:
        return False


def validate_network_target(
    value: str,
    *,
    allowed_origins: Iterable[str] = (),
    allowed_private_hosts: Iterable[str] = (),
    resolver: Callable[..., list[tuple[Any, ...]]] = socket.getaddrinfo,
) -> str:
    """逐次解析目标，默认拒绝 loopback/private/link-local/metadata 地址。"""

    parsed = _parse_http_url(value)
    origin = url_origin(value)
    normalized_origins = {url_origin(item) for item in allowed_origins if item}
    if normalized_origins and origin not in normalized_origins:
        raise CaptureURLSecurityError("采集目标 origin 不在允许列表")
    hostname = (parsed.hostname or "").rstrip(".").lower()
    private_hosts = {item.rstrip(".").lower() for item in allowed_private_hosts if item}
    if hostname in private_hosts:
        return origin
    addresses: set[ipaddress.IPv4Address | ipaddress.IPv6Address] = set()
    try:
        addresses.add(ipaddress.ip_address(hostname))
    except ValueError:
        try:
            for item in resolver(hostname, parsed.port or (443 if parsed.scheme == "https" else 80), type=socket.SOCK_STREAM):
                addresses.add(ipaddress.ip_address(item[4][0]))
        except (OSError, ValueError) as error:
            raise CaptureURLSecurityError("采集目标 DNS 解析失败") from error
    if not addresses or any(_is_forbidden_address(address) for address in addresses):
        raise CaptureURLSecurityError("采集目标解析到不允许的网络地址")
    return origin


def _parse_http_url(value: str):
    if not value or _CONTROL.search(value) or "%00" in value.lower():
        raise CaptureURLSecurityError("采集 URL 无效")
    try:
        parsed = urlsplit(value)
        port = parsed.port
    except ValueError as error:
        raise CaptureURLSecurityError("采集 URL 无效") from error
    if parsed.scheme.lower() not in {"http", "https"} or not parsed.hostname:
        raise CaptureURLSecurityError("采集 URL 必须是 HTTP(S) 地址")
    if port is not None and not 0 < port < 65536:
        raise CaptureURLSecurityError("采集 URL 端口无效")
    return parsed


def _safe_netloc(parsed: Any) -> str:
    host = (parsed.hostname or "").lower()
    netloc = f"[{host}]" if ":" in host and not host.startswith("[") else host
    if parsed.port is not None:
        netloc = f"{netloc}:{parsed.port}"
    return netloc


def _is_forbidden_address(address: ipaddress.IPv4Address | ipaddress.IPv6Address) -> bool:
    return bool(
        address.is_private
        or address.is_loopback
        or address.is_link_local
        or address.is_multicast
        or address.is_reserved
        or address.is_unspecified
    )


def _looks_high_entropy_token(value: str) -> bool:
    if len(value) < 24 or not re.fullmatch(r"[A-Za-z0-9_+/=-]+", value):
        return False
    entropy = -sum(
        (value.count(char) / len(value)) * math.log2(value.count(char) / len(value))
        for char in set(value)
    )
    return entropy >= 3.5
