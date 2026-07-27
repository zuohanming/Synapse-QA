import json
import re
import socket
import time
import urllib.error
import urllib.parse
import urllib.request
import zlib
from ipaddress import ip_address
from typing import Callable

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class ApiRunner(Runner):
    """执行受限 HTTP 调试请求，并实时报告请求与响应阶段。"""

    def run(self, task: TaskCreate, progress: Callable | None = None, canceled: Callable[[], bool] | None = None) -> TaskResult:
        method = str(task.payload.get("method", "GET")).upper()
        url = str(task.payload.get("url") or "")
        if not url:
            raise ValueError("api 任务必须提供 payload.url")
        self._validate_target(url)

        headers = {str(key): str(value) for key, value in (task.payload.get("headers") or {}).items()}
        body = task.payload.get("body")
        timeout = max(1, min(300, int(task.payload.get("timeoutSeconds") or settings.default_timeout_seconds)))
        max_bytes = min(20 << 20, int(task.payload.get("maxResponseBytes") or (20 << 20)))
        data = body.encode("utf-8") if isinstance(body, str) and body else None
        request = urllib.request.Request(url, data=data, headers=headers, method=method)
        started = time.perf_counter()

        if progress:
            progress("request.built", "request", "最终请求已构建", 20, {"method": method, "url": url, "sequence": 2})
            progress("request.connecting", "request", "正在连接目标服务", 35, {"sequence": 3})
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                if progress:
                    progress("response.headers", "response", "已收到响应头", 60, {"statusCode": response.status, "sequence": 4})
                raw = self._read_response(response, max_bytes, canceled)
                if len(raw) > max_bytes:
                    raise ValueError("API_RESPONSE_TOO_LARGE")
                result = self._result(response.status, response.headers, raw, started, max_bytes)
        except urllib.error.HTTPError as error:
            raw = self._read_response(error, max_bytes, canceled)
            if len(raw) > max_bytes:
                raise ValueError("API_RESPONSE_TOO_LARGE")
            if progress:
                progress("response.headers", "response", "已收到响应头", 60, {"statusCode": error.code, "sequence": 4})
            result = self._result(error.code, error.headers, raw, started, max_bytes)

        if progress:
            progress("response.completed", "response", "响应接收完成", 90, {"statusCode": result["statusCode"], "durationMs": result["durationMs"], "sequence": 5})
        variables, extractor_errors, next_sequence = self._run_extractors(task.payload.get("extractors") or [], result, progress, 6)
        assertions, passed = self._run_assertions(task.payload.get("assertions") or [], result, variables, progress, next_sequence)
        result["extractedVariables"] = variables
        result["extractorErrors"] = extractor_errors
        result["assertions"] = assertions
        succeeded = passed and not extractor_errors
        error = "" if succeeded else "响应提取失败" if extractor_errors else "存在未通过的断言"
        return TaskResult(exit_code=0 if succeeded else 1, output=json.dumps(result, ensure_ascii=False), error=error)

    @staticmethod
    def _read_response(response, max_bytes: int, canceled: Callable[[], bool] | None) -> bytes:
        chunks: list[bytes] = []
        size = 0
        while True:
            if canceled and canceled():
                raise InterruptedError("任务已取消")
            chunk = response.read(min(64 << 10, max_bytes + 1 - size))
            if not chunk:
                return b"".join(chunks)
            chunks.append(chunk)
            size += len(chunk)
            if size > max_bytes:
                raise ValueError("API_RESPONSE_TOO_LARGE")

    @staticmethod
    def _result(status: int, headers, raw: bytes, started: float, max_bytes: int) -> dict:
        decoded = ApiRunner._decode_content(raw, ApiRunner._header(dict(headers.items()), "Content-Encoding"), max_bytes)
        return {
            "statusCode": status,
            "headers": dict(headers.items()),
            "body": ApiRunner._decode_text(decoded[: 1 << 20], ApiRunner._header(dict(headers.items()), "Content-Type")),
            "responseSize": len(decoded),
            "compressedResponseSize": len(raw) if len(decoded) != len(raw) else None,
            "truncated": len(decoded) > (1 << 20),
            "durationMs": round((time.perf_counter() - started) * 1000),
        }

    @staticmethod
    def _decode_content(raw: bytes, content_encoding: str | None, max_bytes: int) -> bytes:
        decoded = raw
        encodings = [item.strip().lower() for item in (content_encoding or "").split(",") if item.strip()]
        for encoding in reversed(encodings):
            if encoding in {"", "identity"}:
                continue
            if encoding == "gzip":
                decoded = ApiRunner._zlib_decompress(decoded, zlib.MAX_WBITS | 16, max_bytes)
            elif encoding == "deflate":
                try:
                    decoded = ApiRunner._zlib_decompress(decoded, zlib.MAX_WBITS, max_bytes)
                except zlib.error:
                    decoded = ApiRunner._zlib_decompress(decoded, -zlib.MAX_WBITS, max_bytes)
            else:
                raise ValueError(f"API_RESPONSE_ENCODING_UNSUPPORTED:{encoding}")
        return decoded

    @staticmethod
    def _zlib_decompress(raw: bytes, window_bits: int, max_bytes: int) -> bytes:
        decoder = zlib.decompressobj(window_bits)
        decoded = decoder.decompress(raw, max_bytes + 1)
        if len(decoded) > max_bytes or decoder.unconsumed_tail:
            raise ValueError("API_RESPONSE_TOO_LARGE")
        decoded += decoder.flush(max_bytes + 1 - len(decoded))
        if len(decoded) > max_bytes:
            raise ValueError("API_RESPONSE_TOO_LARGE")
        return decoded

    @staticmethod
    def _decode_text(raw: bytes, content_type: str | None) -> str:
        charset_match = re.search(r"charset\s*=\s*[\"']?([^;\"'\s]+)", content_type or "", re.IGNORECASE)
        declared = charset_match.group(1).strip() if charset_match else "utf-8"
        candidates = [declared, "utf-8", "gb18030"]
        for charset in dict.fromkeys(candidates):
            try:
                return raw.decode(charset)
            except (LookupError, UnicodeDecodeError):
                continue
        return raw.decode("utf-8", errors="replace")

    @staticmethod
    def _validate_target(url: str) -> None:
        parsed = urllib.parse.urlparse(url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("仅允许 HTTP/HTTPS 目标")
        for info in socket.getaddrinfo(parsed.hostname, parsed.port or (443 if parsed.scheme == "https" else 80)):
            address = ip_address(info[4][0])
            if address.is_loopback or address.is_link_local or address.is_multicast or address.is_unspecified:
                raise ValueError("API_SSRF_TARGET_REJECTED")

    def _run_extractors(self, rules: list[dict], result: dict, progress: Callable | None, sequence: int) -> tuple[dict, list[dict], int]:
        variables: dict = {}
        errors: list[dict] = []
        enabled = [rule for rule in rules if rule.get("enabled", True)]
        for index, rule in enumerate(enabled):
            name = str(rule.get("name") or "").strip()
            extractor_type = str(rule.get("type") or rule.get("extractorType") or "").lower()
            required = bool(rule.get("required", True))
            sensitive = bool(rule.get("sensitive", False))
            try:
                if not name:
                    raise ValueError("提取变量名称不能为空")
                value = self._extract_value(extractor_type, rule, result)
                if value is None and required:
                    raise ValueError("未提取到值")
                if value is None:
                    value = rule.get("defaultValue", "")
                variables[name] = value
                message = f"变量 {name} 提取完成"
                event_data = {"name": name, "value": "******" if sensitive else value}
            except Exception as error:
                if required:
                    errors.append({"name": name, "error": str(error)})
                    message = f"变量 {name or index + 1} 提取失败"
                else:
                    variables[name] = rule.get("defaultValue", "")
                    message = f"变量 {name} 使用默认值"
                event_data = {"name": name, "warning": str(error)}
            if progress:
                percent = 90 + round((index + 1) / max(1, len(enabled)) * 3)
                progress("extractor.completed", "extractor", message, percent, {**event_data, "sequence": sequence})
            sequence += 1
        return variables, errors, sequence

    def _extract_value(self, extractor_type: str, rule: dict, result: dict):
        expression = str(rule.get("expression") or "")
        if extractor_type == "jsonpath":
            payload = json.loads(result.get("body") or "null")
            return self._json_path(payload, expression)
        if extractor_type == "regex":
            match = re.search(expression, result.get("body") or "")
            if not match:
                return None
            group = int(rule.get("group", 1 if match.lastindex else 0))
            return match.group(group)
        if extractor_type == "header":
            return self._header(result.get("headers") or {}, expression)
        if extractor_type == "status":
            return result.get("statusCode")
        raise ValueError("不支持的提取类型")

    def _run_assertions(self, rules: list[dict], result: dict, variables: dict, progress: Callable | None, sequence: int) -> tuple[list[dict], bool]:
        details: list[dict] = []
        enabled = [rule for rule in rules if rule.get("enabled", True)]
        for index, rule in enumerate(enabled):
            assertion_type = str(rule.get("type") or rule.get("assertionType") or rule.get("source") or "body").lower()
            expression = str(rule.get("expression") or "")
            operator = str(rule.get("operator") or "equals").lower()
            expected = rule.get("expectedValue", rule.get("expected", ""))
            actual = self._assertion_actual(assertion_type, expression, result, variables)
            passed, error = self._compare(actual, operator, expected)
            detail = {
                "index": index + 1,
                "type": assertion_type,
                "expression": expression,
                "operator": operator,
                "expected": expected,
                "actual": actual,
                "passed": passed,
                "description": str(rule.get("description") or ""),
                "error": error,
            }
            details.append(detail)
            if progress:
                percent = 94 + round((index + 1) / max(1, len(enabled)) * 5)
                progress(
                    "assertion.completed", "assertion",
                    f"断言 {index + 1}{'通过' if passed else '失败'}", percent,
                    {"assertion": detail, "sequence": sequence},
                )
            sequence += 1
        return details, all(item["passed"] for item in details)

    def _assertion_actual(self, assertion_type: str, expression: str, result: dict, variables: dict):
        if assertion_type in {"status", "status_code"}:
            return result.get("statusCode")
        if assertion_type in {"duration", "response_time"}:
            return result.get("durationMs")
        if assertion_type == "header":
            return self._header(result.get("headers") or {}, expression)
        if assertion_type == "jsonpath":
            return self._json_path(json.loads(result.get("body") or "null"), expression)
        if assertion_type == "regex":
            match = re.search(expression, result.get("body") or "")
            return match.group(0) if match else None
        if assertion_type in {"variable", "extracted_variable"}:
            return variables.get(expression)
        return result.get("body") or ""

    @staticmethod
    def _compare(actual, operator: str, expected) -> tuple[bool, str]:
        try:
            if operator == "exists":
                passed = actual is not None
            elif operator == "not_exists":
                passed = actual is None
            elif operator == "equals":
                passed = str(actual) == str(expected)
            elif operator == "not_equals":
                passed = str(actual) != str(expected)
            elif operator == "contains":
                passed = str(expected) in str(actual)
            elif operator == "not_contains":
                passed = str(expected) not in str(actual)
            elif operator == "matches":
                passed = re.search(str(expected), str(actual)) is not None
            elif operator in {"gt", "gte", "lt", "lte"}:
                left, right = float(actual), float(expected)
                passed = {"gt": left > right, "gte": left >= right, "lt": left < right, "lte": left <= right}[operator]
            else:
                return False, f"不支持的操作符：{operator}"
            return passed, "" if passed else "实际值与预期不符"
        except Exception as error:
            return False, str(error)

    @staticmethod
    def _header(headers: dict, name: str):
        for key, value in headers.items():
            if key.lower() == name.lower():
                return value
        return None

    @staticmethod
    def _json_path(payload, expression: str):
        if expression == "$":
            return payload
        if not expression.startswith("$"):
            raise ValueError("JSONPath 必须以 $ 开头")
        tokens = re.findall(r"\.([A-Za-z_][A-Za-z0-9_-]*)|\[(\d+)\]|\['([^']+)'\]|\[\"([^\"]+)\"\]", expression[1:])
        consumed = "$" + "".join(
            f".{dot}" if dot else f"[{index}]" if index else f"['{single or double}']"
            for dot, index, single, double in tokens
        )
        if consumed != expression:
            raise ValueError("首版仅支持属性和数组下标 JSONPath")
        current = payload
        for dot, index, single, double in tokens:
            key = dot or single or double
            current = current[int(index)] if index else current[key]
        return current
