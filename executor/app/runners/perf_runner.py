import json
from collections import deque
import hashlib
import logging
import math
import os
import re
import signal
import subprocess
import tempfile
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable

from app.core.config import settings
from app.core.subprocess_utils import find_k6_executable, hidden_subprocess_kwargs
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


logger = logging.getLogger(__name__)

# 脚本生成器版本，随启动回调上报为 generator_version（SPEC §3.2）。
GENERATOR_VERSION = "perf-1.0.0"

# k6 duration 字符串解析（1m / 30s / 1h / 10m30s 等）。
_DURATION_PATTERN = re.compile(r"^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$")

# 阈值运算符白名单，防注入（SPEC §6）。
_ALLOWED_THRESHOLD_OPERATORS = {"<", ">", "<=", ">=", "==", "!="}

# k6 summaryTrendStats 固定集合（SPEC §4.3）。
_SUMMARY_TREND_STATS = ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"]
_MAX_VUS = 500

# 敏感变量引用 {{secret.xxx}}，渲染为 __ENV.XXX 运行时注入（SPEC §3.4）。
_SECRET_PATTERN = re.compile(r"\{\{\s*secret\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}")
_SENSITIVE_KEY_PATTERN = re.compile(r"(?i)(authorization|cookie|password|passwd|token|secret|api[_-]?key)")
_SENSITIVE_TEXT_PATTERN = re.compile(r"(?i)([\"']?(?:authorization|cookie|password|passwd|token|secret|api[_-]?key)[\"']?\s*[:=]\s*(?:(?:bearer|basic)\s+)?)[^\"'\s,;}]+")
_SENSITIVE_QUERY_PATTERN = re.compile(r"(?i)([?&](?:authorization|cookie|password|passwd|token|secret|api[_-]?key)=)[^&#\s]+")
_SMOKE_RESPONSE_FILE = "smoke-response.json"
_SMOKE_RESPONSE_MARKER = "__SYNAPSE_SMOKE_RESPONSE__"
_SMOKE_BODY_LIMIT = 256 * 1024
_BINARY_CONTENT_TYPE_PATTERN = re.compile(
    r"(?i)(?:application/(?:octet-stream|pdf|zip|gzip|x-7z-compressed|x-rar-compressed)|image/|audio/|video/|font/)"
)


def parse_duration(value: str) -> float:
    """把 k6 duration 字符串解析为秒数。"""
    text = str(value or "").strip()
    if not _DURATION_PATTERN.fullmatch(text):
        raise ValueError(f"无效的时长：{value}")
    total = 0.0
    for number, unit in re.findall(r"(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)", text):
        amount = float(number)
        total += amount * {"ns": 0.000000001, "us": 0.000001, "µs": 0.000001, "ms": 0.001, "s": 1, "m": 60, "h": 3600}[unit]
    if total < 0:
        raise ValueError(f"无效的时长：{value}")
    return total


def total_duration_seconds(scenario_type: str, load_config: dict) -> float:
    """按场景类型计算负载总时长（秒），用于超时动态计算（SPEC §5.1）。"""
    scenario_type = str(scenario_type or "baseline").lower()
    load_config = load_config or {}
    if scenario_type == "smoke":
        return 1.0
    if scenario_type in ("baseline", "soak"):
        return parse_duration(load_config.get("duration") or "1m")
    if scenario_type == "ramp":
        stages = load_config.get("stages") or []
        if not stages:
            raise ValueError("ramp 场景必须提供 load_config.stages")
        return sum(parse_duration(str(stage.get("duration") or "0s")) for stage in stages)
    if scenario_type == "peak":
        return (
            parse_duration(load_config.get("rampDuration") or "1m")
            + parse_duration(load_config.get("holdDuration") or "10m")
            + parse_duration(load_config.get("rampDownDuration") or "1m")
        )
    if scenario_type == "stress":
        start_vus = int(load_config.get("startVus") or 1)
        step_vus = max(1, int(load_config.get("stepVus") or 1))
        max_vus = int(load_config.get("maxVus") or start_vus)
        step_duration = parse_duration(load_config.get("stepDuration") or "1m")
        steps = 1 + max(0, (max_vus - start_vus + step_vus - 1) // step_vus)
        return steps * step_duration
    if scenario_type == "mixed":
        return parse_duration(load_config.get("duration") or "1m")
    raise ValueError(f"不支持的场景类型：{scenario_type}")


def compute_timeout(scenario_type: str, load_config: dict) -> float:
    """动态计算超时上限 = 总时长 × 系数 + 缓冲（SPEC §5.1）。"""
    return (
        total_duration_seconds(scenario_type, load_config) * settings.perf_timeout_coefficient
        + settings.perf_timeout_buffer_seconds
    )


def render_options(scenario_type: str, load_config: dict) -> dict:
    """渲染 k6 options（SPEC §2.2）。"""
    scenario_type = str(scenario_type or "baseline").lower()
    load_config = load_config or {}
    if scenario_type == "smoke":
        return {"scenarios": {"smoke": {"executor": "shared-iterations", "iterations": 1, "vus": 1}}}
    if scenario_type in ("baseline", "soak"):
        _validate_vus(load_config.get("vus") or 1)
        return {
            "vus": int(load_config.get("vus") or 1),
            "duration": str(load_config.get("duration") or "1m"),
        }
    if scenario_type == "ramp":
        stages = load_config.get("stages") or []
        if not stages:
            raise ValueError("ramp 场景必须提供 load_config.stages")
        options = {
            "executor": "ramping-vus",
            "stages": [
                {"duration": str(stage.get("duration") or "0s"), "target": int(stage.get("target") or 0)}
                for stage in stages
            ],
        }
        _validate_stage_targets(options["stages"])
        return options
    if scenario_type == "peak":
        peak_vus = int(load_config.get("peakVus") or 0)
        _validate_vus(peak_vus)
        return {
            "executor": "ramping-vus",
            "stages": [
                {"duration": str(load_config.get("rampDuration") or "1m"), "target": peak_vus},
                {"duration": str(load_config.get("holdDuration") or "10m"), "target": peak_vus},
                {"duration": str(load_config.get("rampDownDuration") or "1m"), "target": 0},
            ],
        }
    if scenario_type == "stress":
        start_vus = int(load_config.get("startVus") or 1)
        step_vus = max(1, int(load_config.get("stepVus") or 1))
        max_vus = int(load_config.get("maxVus") or start_vus)
        _validate_vus(start_vus)
        _validate_vus(max_vus)
        step_duration = str(load_config.get("stepDuration") or "1m")
        stages = []
        current = start_vus
        stages.append({"duration": step_duration, "target": current})
        while current < max_vus:
            current = min(max_vus, current + step_vus)
            stages.append({"duration": step_duration, "target": current})
        _validate_stage_targets(stages)
        return {"executor": "ramping-vus", "stages": stages}
    if scenario_type == "mixed":
        scenarios = load_config.get("scenarios") or []
        _validate_vus(load_config.get("vus") or 1)
        _validate_mixed_scenarios(scenarios)
        return {"scenarios": {"mixedFlow": {"executor": "constant-vus", "vus": int(load_config.get("vus") or 1), "duration": str(load_config.get("duration") or "1m"), "exec": "mixedFlow"}}}
    raise ValueError(f"不支持的场景类型：{scenario_type}")


def _validate_vus(value) -> None:
    try:
        vus = int(value)
    except (TypeError, ValueError):
        raise ValueError("vus 必须是正整数")
    if vus <= 0 or vus > _MAX_VUS:
        raise ValueError(f"vus 不能超过 {_MAX_VUS}")


def _validate_stage_targets(stages: list[dict]) -> None:
    for stage in stages:
        _validate_vus(stage.get("target") or 0)


def _is_absolute_http_url(value: str) -> bool:
    return bool(re.match(r"^https?://[^\s/]+(?:/[^\s]*)?$", value, re.IGNORECASE))


def _downsample_series_points(points: list[dict], limit: int = 3000) -> list[dict]:
    if not points or limit <= 0:
        return []
    if len(points) <= limit:
        return points
    required = {0, len(points) - 1}
    for index in range(1, len(points)):
        if points[index - 1].get("stage") != points[index].get("stage"):
            required.update({index - 1, index})
    if limit == 1:
        return [points[0]]

    def uniformly_select(candidates: list[int], count: int) -> set[int]:
        if count <= 0:
            return set()
        if count >= len(candidates):
            return set(candidates)
        if count == 1:
            return {candidates[(len(candidates) - 1) // 2]}
        return {
            candidates[position * (len(candidates) - 1) // (count - 1)]
            for position in range(count)
        }

    if len(required) > limit:
        # 预算不足时只在完整的边界集合上均匀取样，避免排序后截取前缀丢失尾部阶段。
        indices = uniformly_select(sorted(required), limit)
    else:
        # 预算足够时先保留所有阶段边界，再从其余点的全程均匀采样。
        candidates = [index for index in range(len(points)) if index not in required]
        indices = required | uniformly_select(candidates, limit - len(required))
    indices = sorted(indices)
    return [points[index] for index in indices]


def render_thresholds(thresholds: list) -> dict[str, list[dict]]:
    """把平台结构化阈值数组渲染为 k6 thresholds 映射（SPEC §3.1）。

    形如 [{"metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 500}]
    渲染为 {"http_req_duration": ["p(95)<500"]}。
    """
    result: dict[str, list[dict]] = {}
    for item in thresholds or []:
        if not isinstance(item, dict):
            continue
        metric = str(item.get("metric") or "").strip()
        aggregation = str(item.get("aggregation") or "").strip()
        operator = str(item.get("operator") or "").strip()
        value = item.get("value")
        if not metric or not aggregation:
            continue
        if operator not in _ALLOWED_THRESHOLD_OPERATORS:
            logger.warning("忽略不支持的阈值运算符：%s", operator)
            continue
        if value is None or value == "":
            logger.warning("忽略缺少 value 的阈值：metric=%s", metric)
            continue
        threshold = {
            "threshold": f"{aggregation}{operator}{value}",
            "abortOnFail": bool(item.get("abortOnFail")),
        }
        delay = str(item.get("delayAbortEval") or "").strip()
        if threshold["abortOnFail"] and delay:
            parse_duration(delay)
            threshold["delayAbortEval"] = delay
        result.setdefault(metric, []).append(threshold)
    return result


def _validate_mixed_scenarios(scenarios: list) -> None:
    if not isinstance(scenarios, list) or not scenarios:
        raise ValueError("mixed 场景必须提供 scenarios")
    total = 0.0
    names: set[str] = set()
    for item in scenarios:
        if not isinstance(item, dict):
            raise ValueError("mixed 场景接口配置无效")
        name = str(item.get("name") or "").strip()
        url = str(item.get("url") or "").strip()
        weight = item.get("weight")
        try:
            weight = float(str(weight))
        except (TypeError, ValueError):
            weight = 0
        if not name or name in names or not url or weight <= 0:
            raise ValueError("mixed 场景名称、权重和 URL 必须有效且名称唯一")
        headers = item.get("headers", {})
        if headers is not None and not isinstance(headers, dict):
            raise ValueError("mixed 场景 headers 必须是对象")
        if "body" in item and not isinstance(item["body"], (str, int, float, bool, list, dict, type(None))):
            raise ValueError("mixed 场景 body 类型无效")
        names.add(name)
        total += weight
    if abs(total - 1.0) > 1e-6:
        raise ValueError("mixed 场景权重总和必须为 1")


def _render_js_string(value: str, secret_envs: dict[str, str]) -> str:
    """把字符串渲染为合法 JS 表达式，支持字符串内嵌 secret 引用。"""
    text = str(value)
    matches = list(_SECRET_PATTERN.finditer(text))
    if not matches:
        return json.dumps(text, ensure_ascii=False)
    parts: list[str] = []
    cursor = 0
    for match in matches:
        if match.start() > cursor:
            parts.append(json.dumps(text[cursor:match.start()], ensure_ascii=False))
        name = match.group(1)
        secret_envs[name] = name
        parts.append(f"__ENV.{name}")
        cursor = match.end()
    if cursor < len(text):
        parts.append(json.dumps(text[cursor:], ensure_ascii=False))
    return " + ".join(parts)


def _render_headers(headers: dict, secret_envs: dict[str, str]) -> str:
    if not headers:
        return "{}"
    pairs = []
    for key, value in headers.items():
        key_js = json.dumps(str(key), ensure_ascii=False)
        value_js = _render_js_value(value, secret_envs)
        pairs.append(f"{key_js}: {value_js}")
    return "{" + ", ".join(pairs) + "}"


def _render_body(body, secret_envs: dict[str, str]) -> str:
    return _render_js_value(body, secret_envs)


def _render_js_value(value, secret_envs: dict[str, str]) -> str:
    if isinstance(value, str):
        return _render_js_string(value, secret_envs)
    if isinstance(value, dict):
        return "{" + ", ".join(
            json.dumps(str(key), ensure_ascii=False) + ": " + _render_js_value(item, secret_envs)
            for key, item in value.items()
        ) + "}"
    if isinstance(value, list):
        return "[" + ", ".join(_render_js_value(item, secret_envs) for item in value) + "]"
    return json.dumps(value, ensure_ascii=False)


def _render_mixed_scenarios(scenarios: list, secret_envs: dict[str, str]) -> str:
    rendered = []
    for item in scenarios:
        fields = [
            json.dumps("name") + ": " + json.dumps(str(item.get("name") or ""), ensure_ascii=False),
            json.dumps("weight") + ": " + json.dumps(float(str(item.get("weight")))),
            json.dumps("method") + ": " + json.dumps(str(item.get("method") or "GET").upper()),
            json.dumps("url") + ": " + _render_js_string(str(item.get("url") or ""), secret_envs),
            json.dumps("headers") + ": " + _render_headers(item.get("headers") or {}, secret_envs),
            json.dumps("body") + ": " + _render_body(item.get("body"), secret_envs),
        ]
        rendered.append("{" + ", ".join(fields) + "}")
    return "[" + ", ".join(rendered) + "]"


def generate_script(payload: dict) -> tuple[str, dict[str, str]]:
    """渲染 k6 脚本，返回 (脚本文本, 敏感环境变量映射)。"""
    scenario_type = "smoke" if payload.get("mode") == "smoke" else str(payload.get("scenario_type") or "baseline").lower()
    load_config = payload.get("load_config") or {}

    options = render_options(scenario_type, load_config)
    thresholds = render_thresholds(payload.get("thresholds") or [])
    options["summaryTrendStats"] = _SUMMARY_TREND_STATS
    if thresholds:
        options["thresholds"] = thresholds

    target = str(payload.get("target") or payload.get("url") or "").strip()
    method = str(payload.get("method") or "GET").upper()

    secret_envs: dict[str, str] = {}
    target_js = _render_js_string(target, secret_envs)
    headers_js = _render_headers(payload.get("headers") or {}, secret_envs)
    body_js = _render_body(payload.get("body"), secret_envs)

    options_js = json.dumps(options, ensure_ascii=False)
    if scenario_type == "mixed":
        scenarios = load_config.get("scenarios") or []
        _validate_mixed_scenarios(scenarios)
        if any(not _is_absolute_http_url(str(item.get("url") or "")) for item in scenarios) and not _is_absolute_http_url(target):
            raise ValueError("mixed 含相对 URL 时必须提供合法 HTTP(S) payload.target")
        mixed_scenarios = _render_mixed_scenarios(scenarios, secret_envs)
        think_time = json.dumps(parse_duration(str(load_config.get("thinkTime") or "0s")))
        flow = (
            "const mixedScenarios = " + mixed_scenarios + ";\n"
            "function resolveMixedTarget(path) {\n"
            "  const value = String(path || '');\n"
            "  if (value.indexOf('://') > 0) return value;\n"
            "  const schemeEnd = target.indexOf('://');\n"
            "  const originEnd = schemeEnd >= 0 ? target.indexOf('/', schemeEnd + 3) : -1;\n"
            "  const origin = originEnd >= 0 ? target.slice(0, originEnd) : target.replace(/\\/+$/, '');\n"
            "  if (value.startsWith('/')) return origin + value;\n"
            "  const slash = target.lastIndexOf('/');\n"
            "  const base = slash > (schemeEnd + 2) ? target.slice(0, slash + 1) : target + '/';\n"
            "  return base + value.replace(/^\\.\\//, '');\n"
            "}\n"
            "export function mixedFlow() {\n"
            "  const roll = Math.random();\n"
            "  let cursor = 0;\n"
            "  let selected = null;\n"
            "  for (const item of mixedScenarios) { cursor += Number(item.weight || 0); if (roll < cursor) { selected = item; break; } }\n"
            "  if (!selected) throw new Error('mixed weight distribution is invalid');\n"
            "  const requestTarget = resolveMixedTarget(selected.url);\n"
            "  const requestHeaders = Object.assign({}, headers, selected.headers || {});\n"
            "  http.request(String(selected.method || method).toUpperCase(), requestTarget, selected.body ?? body, { headers: requestHeaders, tags: { scenario: selected.name } });\n"
            "  sleep(" + think_time + ");\n"
            "}\n"
        )
        imports = "import http from 'k6/http';\nimport { sleep } from 'k6';\n"
    elif scenario_type == "smoke":
        if not target:
            raise ValueError("perf 任务必须提供 payload.target")
        flow = (
            "let smokeResponse = null;\n"
            "function responseHeader(headers, name) {\n"
            "  for (const key in (headers || {})) { if (String(key).toLowerCase() === name) return headers[key]; }\n"
            "  return '';\n"
            "}\n"
            "function sensitiveResponseHeader(name) {\n"
            "  const normalized = String(name || '').toLowerCase().replace(/_/g, '-');\n"
            "  return ['set-cookie', 'cookie', 'authorization', 'proxy-authorization', 'x-api-key'].indexOf(normalized) >= 0 || /token|secret|password|api-key/.test(normalized);\n"
            "}\n"
            "function utf8ByteLength(value) {\n"
            "  try { return encodeURIComponent(value).replace(/%[0-9a-f]{2}/gi, 'x').length; } catch (_) { return String(value).length * 3; }\n"
            "}\n"
            f"const maxSmokeBodyBytes = {_SMOKE_BODY_LIMIT};\n"
            "function truncateUtf8(value, maxBytes) {\n"
            "  let result = '';\n"
            "  let size = 0;\n"
            "  for (const character of Array.from(value)) {\n"
            "    const characterSize = utf8ByteLength(character);\n"
            "    if (size + characterSize > maxBytes) break;\n"
            "    result += character;\n"
            "    size += characterSize;\n"
            "  }\n"
            "  return result;\n"
            "}\n"
            "function binaryContentType(value) {\n"
            "  return /(?:application\\/(?:octet-stream|pdf|zip|gzip|x-7z-compressed|x-rar-compressed)|image\\/|audio\\/|video\\/|font\\/)/i.test(String(value || ''));\n"
            "}\n"
            "function captureSmokeResponse(response) {\n"
            "  const responseBody = response.body == null ? '' : String(response.body);\n"
            "  const contentType = String(responseHeader(response.headers, 'content-type') || '');\n"
            "  const rawBodySize = utf8ByteLength(responseBody);\n"
            "  const isBinary = binaryContentType(contentType) || responseBody.indexOf('\\u0000') >= 0;\n"
            "  const truncated = !isBinary && rawBodySize > maxSmokeBodyBytes;\n"
            "  const safeBody = isBinary ? '' : (truncated ? truncateUtf8(responseBody, maxSmokeBodyBytes) : responseBody);\n"
            "  const responseHeaders = {};\n"
            "  for (const key in (response.headers || {})) responseHeaders[key] = sensitiveResponseHeader(key) ? '******' : response.headers[key];\n"
            "  smokeResponse = { statusCode: Number(response.status), headers: responseHeaders, body: safeBody, durationMs: Number((response.timings || {}).duration || 0), truncated: truncated, bodySize: rawBodySize, isBinary: isBinary, contentType: contentType };\n"
            f"  console.log({json.dumps(_SMOKE_RESPONSE_MARKER)} + JSON.stringify(smokeResponse));\n"
            "}\n"
            "export default function () {\n"
            "  const response = http.request(method, target, body, { headers: headers });\n"
            "  captureSmokeResponse(response);\n"
            "}\n"
        )
        imports = "import http from 'k6/http';\n"
    else:
        if not target:
            raise ValueError("perf 任务必须提供 payload.target")
        flow = "export default function () {\n  http.request(method, target, body, { headers: headers });\n}\n"
        imports = "import http from 'k6/http';\n"
    summary_files = "    'report.json': JSON.stringify(data),\n"
    if scenario_type == "smoke":
        summary_files += f"    '{_SMOKE_RESPONSE_FILE}': JSON.stringify(smokeResponse || {{}}),\n"
    script = (
        "// 由 Synapse QA 执行器生成，generator_version=" + GENERATOR_VERSION + "\n"
        + imports
        + "\n"
        + "export const options = " + options_js + ";\n"
        + "\n"
        + "const target = " + target_js + ";\n"
        + "const method = " + json.dumps(method, ensure_ascii=False) + ";\n"
        + "const headers = " + headers_js + ";\n"
        + "const body = " + body_js + ";\n"
        + "\n"
        + flow
        + "\n"
        + "export function handleSummary(data) {\n"
        + "  return {\n"
        + summary_files
        + "  };\n"
        + "}\n"
    )
    return script, secret_envs


class NdjsonSampler:
    """消费 k6 `--out json` 的 NDJSON，聚合为短周期采样并定期原子写快照（SPEC §7.1）。"""

    def __init__(
        self,
        ndjson_path: Path,
        snapshot_path: Path,
        task_id: str = "",
        scenario_type: str = "baseline",
        load_config: dict | None = None,
        thresholds: list | None = None,
        sample_interval_ms: int = 2000,
        sample_callback: Callable[[dict], None] | None = None,
        clock: Callable[[], float] | None = None,
    ) -> None:
        self._ndjson_path = ndjson_path
        self._snapshot_path = snapshot_path
        self._interval = settings.perf_snapshot_interval_seconds
        self._task_id = task_id
        self._scenario_type = scenario_type
        self._load_config = load_config or {}
        self._thresholds = thresholds or []
        self._sample_interval_ms = max(1000, min(5000, int(sample_interval_ms or 2000)))
        self._sample_callback = sample_callback
        self._clock = clock or time.monotonic
        self._sequence = 0
        self._points: list[dict] = []
        self._last_event_at = self._clock()
        self._status_codes: dict[str, int] = {}
        self._error_counts: dict[str, dict[str, object]] = {}
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._lock = threading.Lock()
        self._count = 0
        self._window_seconds = self._sample_interval_ms / 1000
        self._hard_window_limit = 10000
        self._window_limit = max(1, min(self._hard_window_limit, int(settings.perf_ndjson_window_points)))
        self._durations: deque[tuple[float, float]] = deque(maxlen=self._window_limit)
        self._recent_requests: deque[tuple[float, int]] = deque(maxlen=self._window_limit)
        self._recent_failures: deque[tuple[float, bool]] = deque(maxlen=self._window_limit)
        self._metric_windows: dict[str, deque[tuple[float, float]]] = {}
        self._bucket_width_seconds = 0.01
        self._request_buckets: dict[int, int] = {}
        self._failure_buckets: dict[int, list[int]] = {}
        self._duration_buckets: dict[int, list[float]] = {}
        self._duration_bucket_seen: dict[int, int] = {}
        self._duration_reservoir_limit = 500
        self._fails = 0
        self._total = 0
        self._duration_sum = 0.0
        self._duration_count = 0
        self._vus = int(self._load_config.get("vus") or self._load_config.get("startVus") or 0)
        self._vus_max = self._vus
        self._latest_thresholds: list[dict] = []
        self._latest_message = ""
        self._started_at = self._clock()
        self._series_started_at = datetime.now(timezone.utc)
        self._series_ended_at: datetime | None = None

    def start(self) -> None:
        self._thread = threading.Thread(target=self._run, name="perf-ndjson-sampler", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=2)
        self._series_ended_at = datetime.now(timezone.utc)

    def snapshot(self) -> dict:
        with self._lock:
            return self._aggregate()

    def elapsed_seconds(self) -> float:
        return max(0.0, self._clock() - self._started_at)

    def duration_seconds(self) -> float:
        return total_duration_seconds(self._scenario_type, self._load_config)

    def _run(self) -> None:
        offset = 0
        last_flush = self._clock()
        while not self._stop.is_set():
            try:
                with open(self._ndjson_path, "r", encoding="utf-8") as handle:
                    handle.seek(offset)
                    for line in handle:
                        self._consume_line(line)
                    offset = handle.tell()
            except (FileNotFoundError, OSError):
                pass
            if self._clock() - last_flush >= self._interval:
                self._write_snapshot()
                last_flush = self._clock()
            if self._clock() - self._last_event_at >= self._sample_interval_ms / 1000:
                self.emit_sample()
            time.sleep(0.5)
        self._write_snapshot()
        if self._sample_callback:
            self.emit_sample(force=True)

    def _consume_line(self, line: str) -> None:
        text = line.strip()
        if not text:
            return
        try:
            item = json.loads(text)
        except json.JSONDecodeError:
            return
        if item.get("type") != "Point":
            return
        metric = item.get("metric")
        value = (item.get("data") or {}).get("value")
        if not isinstance(value, (int, float)):
            return
        with self._lock:
            now = self._clock()
            if metric == "vus":
                self._vus = int(value)
                self._vus_max = max(self._vus_max, self._vus)
                self._metric_window(metric).append((now, float(value)))
                self._evict_window(now)
                return
            if metric == "vus_max":
                self._vus_max = max(self._vus_max, int(value))
                self._metric_window(metric).append((now, float(value)))
                self._evict_window(now)
                return
            if metric == "http_reqs":
                # k6 --out json 每个 http_reqs Point 代表一次计数，应累计而非覆盖。
                count = int(value)
                self._count += count
                bucket = self._bucket(now)
                self._request_buckets[bucket] = self._request_buckets.get(bucket, 0) + count
                self._recent_requests.append((now, count))
                self._metric_window(metric).append((now, float(count)))
                tags = item.get("data", {}).get("tags") or {}
                status = str(tags.get("status") or "")
                if status:
                    self._status_codes[status] = self._status_codes.get(status, 0) + count
                if status and status not in {"200", "201", "202", "204", "304"}:
                    key = f"status:{status}"
                    entry = self._error_counts.setdefault(key, {"key": key, "message": f"HTTP {status}", "count": 0})
                    entry["count"] = _int_value(entry.get("count", 0)) + count
            elif metric == "http_req_duration":
                duration = float(value)
                bucket = self._bucket(now)
                reservoir = self._duration_buckets.setdefault(bucket, [])
                seen = self._duration_bucket_seen.get(bucket, 0)
                if len(reservoir) < self._duration_reservoir_limit:
                    reservoir.append(duration)
                else:
                    reservoir[seen % self._duration_reservoir_limit] = duration
                self._duration_bucket_seen[bucket] = seen + 1
                self._durations.append((now, duration))
                self._metric_window(metric).append((now, duration))
                self._duration_sum += duration
                self._duration_count += 1
            elif metric == "http_req_failed":
                self._total += 1
                failed = bool(value)
                bucket = self._bucket(now)
                counts = self._failure_buckets.setdefault(bucket, [0, 0])
                counts[0] += 1
                counts[1] += int(failed)
                self._recent_failures.append((now, failed))
                self._metric_window(metric).append((now, 1.0 if failed else 0.0))
                if failed:
                    self._fails += 1
            else:
                self._metric_window(metric).append((now, float(value)))
            self._evict_window(now)

    def _metric_window(self, metric: str) -> deque[tuple[float, float]]:
        window = self._metric_windows.get(metric)
        if window is None:
            window = deque(maxlen=self._window_limit)
            self._metric_windows[metric] = window
        return window

    def _bucket(self, timestamp: float) -> int:
        return int(timestamp / self._bucket_width_seconds)

    def _cutoff_bucket(self) -> int:
        return math.ceil((self._clock() - self._window_seconds) / self._bucket_width_seconds)

    def _evict_window(self, now: float) -> None:
        cutoff = now - self._window_seconds
        for window in (self._durations, self._recent_requests, self._recent_failures, *self._metric_windows.values()):
            while window and window[0][0] < cutoff:
                window.popleft()
        cutoff_bucket = math.ceil(cutoff / self._bucket_width_seconds)
        for buckets in (self._request_buckets, self._failure_buckets, self._duration_buckets, self._duration_bucket_seen):
            for bucket in list(buckets):
                if bucket < cutoff_bucket:
                    del buckets[bucket]

    def _window_duration_values(self) -> list[float]:
        return [value for bucket, values in self._duration_buckets.items() if bucket >= self._cutoff_bucket() for value in values]

    def _window_request_count(self) -> int:
        cutoff = self._cutoff_bucket()
        return sum(value for bucket, value in self._request_buckets.items() if bucket >= cutoff)

    def _window_failure_counts(self) -> tuple[int, int]:
        cutoff = self._cutoff_bucket()
        total = sum(values[0] for bucket, values in self._failure_buckets.items() if bucket >= cutoff)
        failed = sum(values[1] for bucket, values in self._failure_buckets.items() if bucket >= cutoff)
        return total, failed

    def emit_sample(self, force: bool = False) -> None:
        if not self._sample_callback:
            return
        now = self._clock()
        if not force and now - self._last_event_at < self._sample_interval_ms / 1000:
            return
        self._last_event_at = now
        snapshot = self._aggregate()
        self._sequence += 1
        values = sorted(self._window_duration_values())
        window = snapshot.get("window") or {}
        duration_ms = int(total_duration_seconds(self._scenario_type, self._load_config) * 1000)
        elapsed_ms = int((now - self._started_at) * 1000)
        thresholds = []
        messages = []
        for item in self._thresholds:
            metric = str(item.get("metric") or "")
            expression = f"{item.get('aggregation', 'value')}{item.get('operator', '')}{item.get('value', '')}"
            delay = str(item.get("delayAbortEval") or "").strip()
            try:
                delay_seconds = parse_duration(delay) if delay else 0
                delay_error = ""
            except ValueError:
                delay_seconds, delay_error = 0, "delayAbortEval 无效"
            actual, reason = self._threshold_actual(metric, str(item.get("aggregation") or ""), now)
            if delay_error:
                status, reason = "pending", delay_error
            elif elapsed_ms / 1000 < delay_seconds:
                status, reason = "pending", f"等待 delayAbortEval={delay}"
            elif reason:
                status = "pending"
            else:
                status = "passing" if _threshold_passed(actual, item.get("operator"), item.get("value")) else "failing"
            if reason:
                messages.append(f"{metric}/{item.get('aggregation')}: {reason}")
            thresholds.append({"metric": metric, "expression": expression, "actual": actual, "status": status})
        event = {
            "taskId": self._task_id,
            "sequence": self._sequence,
            "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
            "type": "sample",
            "status": "running",
            "stage": self._stage_for(elapsed_ms),
            "remainingMs": max(0, duration_ms - elapsed_ms),
            "windowMs": self._sample_interval_ms,
            "vus": self._vus,
            "totalRequests": self._count,
            "rps": window.get("rps"),
            "p50": self._percentile(values, 0.50),
            "p90": self._percentile(values, 0.90),
            "p95": self._percentile(values, 0.95),
            "p99": self._percentile(values, 0.99),
            "errorRate": window.get("error_rate"),
            "statusCodes": dict(self._status_codes),
            "errorTopN": sorted(self._error_counts.values(), key=lambda item: _int_value(item.get("count", 0)), reverse=True)[:10],
            "thresholds": thresholds,
            "message": "; ".join(messages) if messages else "",
        }
        self._latest_thresholds = thresholds
        self._latest_message = "; ".join(messages)
        self._append_series_point({key: value for key, value in event.items() if key not in {"statusCodes", "errorTopN", "thresholds", "message"}})
        try:
            self._sample_callback(event)
        except Exception:
            logger.exception("性能采样事件回调失败")

    def series(self, partial: bool) -> dict:
        ended_at = self._series_ended_at or datetime.now(timezone.utc)
        return {
            "version": 1,
            "intervalMs": self._sample_interval_ms,
            "lastSequence": self._sequence,
            "startedAt": self._series_started_at.isoformat().replace("+00:00", "Z"),
            "endedAt": ended_at.isoformat().replace("+00:00", "Z"),
            "partial": partial,
            "points": _downsample_series_points(self._points, 3000),
            "statusCodes": dict(self._status_codes),
            "errorTopN": sorted(self._error_counts.values(), key=lambda item: _int_value(item.get("count", 0)), reverse=True)[:10],
            "thresholds": self._latest_thresholds,
        }

    def _append_series_point(self, point: dict) -> None:
        self._points.append(point)
        if len(self._points) > 6000:
            self._points = _downsample_series_points(self._points, 3000)

    def _aggregate(self) -> dict:
        now = self._clock()
        self._evict_window(now)
        durations = sorted(self._window_duration_values())
        elapsed = max(0.001, now - self._started_at)
        window_count = self._window_request_count()
        window_elapsed = max(0.001, self._window_seconds)
        failure_total, failure_count = self._window_failure_counts()
        window_error_rate = (
            failure_count / failure_total
            if failure_total
            else None
        )
        window = {
            "p50_duration_ms": self._percentile(durations, 0.50),
            "p90_duration_ms": self._percentile(durations, 0.90),
            "p95_duration_ms": self._percentile(durations, 0.95),
            "p99_duration_ms": self._percentile(durations, 0.99),
            "rps": round(window_count / window_elapsed, 2),
            "error_rate": round(window_error_rate, 4) if window_error_rate is not None else None,
            "sample_points": len(durations),
            "vus": self._vus,
            "vus_max": self._vus_max,
        }
        return {
            "total_requests": self._count,
            "avg_duration_ms": round(self._duration_sum / self._duration_count) if self._duration_count else None,
            "p95_duration_ms": self._percentile(durations, 0.95),
            "error_rate": round(self._fails / self._total, 4) if self._total else None,
            "rps": round(self._count / elapsed, 2),
            "window": window,
            "partial": True,
        }

    def _threshold_actual(self, metric: str, aggregation: str, now: float) -> tuple[float | None, str]:
        if metric == "http_reqs":
            count = self._window_request_count()
            if aggregation == "count":
                return float(count), "" if count else "暂无样本"
            if aggregation == "rate":
                return count / max(0.001, self._window_seconds), "" if count else "暂无样本"
        if metric == "http_req_failed":
            total, failed = self._window_failure_counts()
            if not total:
                return None, "暂无样本"
            if aggregation == "rate":
                return failed / total, ""
        if metric == "http_req_duration":
            values = self._window_duration_values()
        else:
            values = [value for _, value in self._metric_windows.get(metric, [])]
        if not values:
            return None, "暂无样本"
        if aggregation in {"p(50)", "p(90)", "p(95)", "p(99)"}:
            return self._percentile(sorted(values), float(aggregation[2:-1]) / 100), ""
        if aggregation == "avg":
            return sum(values) / len(values), ""
        if aggregation == "min":
            return min(values), ""
        if aggregation == "max":
            return max(values), ""
        if aggregation == "count":
            return sum(values) if metric in {"http_reqs", "iterations"} else float(len(values)), ""
        if aggregation == "rate":
            if metric == "http_req_failed":
                return sum(values) / len(values), ""
            return sum(values) / max(0.001, self._window_seconds), ""
        return None, "不支持该指标聚合方式"

    def _stage_for(self, elapsed_ms: int) -> str:
        elapsed = elapsed_ms / 1000
        if self._scenario_type in {"baseline", "soak", "mixed"}:
            return "steady"
        if self._scenario_type == "peak":
            ramp = parse_duration(str(self._load_config.get("rampDuration") or "1m"))
            hold = parse_duration(str(self._load_config.get("holdDuration") or "10m"))
            return "ramp-up" if elapsed < ramp else "hold" if elapsed < ramp + hold else "ramp-down"
        stages = self._load_config.get("stages") or []
        if self._scenario_type == "stress":
            step = parse_duration(str(self._load_config.get("stepDuration") or "1m"))
            start = int(self._load_config.get("startVus") or 1)
            step_vus = max(1, int(self._load_config.get("stepVus") or 1))
            max_vus = int(self._load_config.get("maxVus") or start)
            count = 1 + max(0, (max_vus - start + step_vus - 1) // step_vus)
            return f"stage:{min(count, int(elapsed // step) + 1)}"
        if self._scenario_type == "ramp":
            elapsed_total = 0.0
            for index, stage in enumerate(stages, 1):
                elapsed_total += parse_duration(str(stage.get("duration") or "0s"))
                if elapsed < elapsed_total:
                    return f"stage:{index}"
            return f"stage:{len(stages) or 1}"
        return "steady"

    def _write_snapshot(self) -> None:
        payload = json.dumps(self._aggregate(), ensure_ascii=False)
        tmp = self._snapshot_path.with_suffix(".tmp")
        tmp.write_text(payload, encoding="utf-8")
        os.replace(tmp, self._snapshot_path)

    @staticmethod
    def _percentile(sorted_values: list[float], q: float) -> float | None:
        if not sorted_values:
            return None
        index = (len(sorted_values) - 1) * q
        low = int(index)
        high = min(low + 1, len(sorted_values) - 1)
        frac = index - low
        return round(sorted_values[low] * (1 - frac) + sorted_values[high] * frac)


class PerfRunner(Runner):
    """渲染 k6 脚本并调度 k6 子进程执行性能测试（SPEC §4）。"""

    def run(
        self,
        task: TaskCreate,
        progress: Callable | None = None,
        canceled: Callable[[], bool] | None = None,
        started: Callable[[dict], None] | None = None,
        sample: Callable[[dict], None] | None = None,
    ) -> TaskResult:
        payload = task.payload or {}
        scenario_type = "smoke" if payload.get("mode") == "smoke" else str(payload.get("scenario_type") or "baseline").lower()
        load_config = payload.get("load_config") or {}

        if progress:
            progress("[检查] 正在检测 k6 运行环境")
        k6_path = find_k6_executable()
        if not k6_path:
            error = "未安装 k6，无法执行性能测试"
            return self._result(scenario_type, exit_code=1, failure_stage="startup", error=error, partial=False, terminal_status="execution_failed")

        try:
            script, secret_envs = generate_script(payload)
        except ValueError as error:
            return self._result(scenario_type, exit_code=1, failure_stage="script_generation", error=str(error), partial=False, terminal_status="execution_failed")

        script_hash = hashlib.sha256(script.encode("utf-8")).hexdigest()
        k6_version = self._k6_version(k6_path)

        try:
            timeout = compute_timeout(scenario_type, payload.get("load_config") or {})
        except ValueError as error:
            return self._result(scenario_type, exit_code=1, failure_stage="script_generation", error=str(error), partial=False, terminal_status="execution_failed")

        if progress:
            progress(f"[开始] 执行 k6 压测（{scenario_type}，超时上限 {timeout:g}s）")

        with tempfile.TemporaryDirectory(prefix="perf-") as workdir:
            workdir_path = Path(workdir)
            script_path = workdir_path / "script.js"
            script_path.write_text(script, encoding="utf-8")

            result = self._run_k6(
                k6_path,
                workdir_path,
                secret_envs,
                scenario_type,
                timeout,
                canceled,
                progress,
                script_hash,
                k6_version,
                started,
                sample,
                task.task_id or "",
                load_config,
                payload.get("thresholds") or [],
                task.sample_interval_ms,
            )
            if scenario_type == "smoke":
                self._attach_smoke_response(result, workdir_path / _SMOKE_RESPONSE_FILE, workdir_path / "k6.log")
        return self._attach_runtime_metadata(result, script_hash, k6_version)

    def _run_k6(
        self,
        k6_path: str,
        workdir: Path,
        secret_envs: dict[str, str],
        scenario_type: str,
        timeout: float,
        canceled: Callable[[], bool] | None,
        progress: Callable | None,
        script_hash: str = "",
        k6_version: str = "",
        started: Callable[[dict], None] | None = None,
        sample: Callable[[dict], None] | None = None,
        task_id: str = "",
        load_config: dict | None = None,
        thresholds: list | None = None,
        sample_interval_ms: int = 2000,
    ) -> TaskResult:
        ndjson_path = workdir / "ndjson.out"
        report_path = workdir / "report.json"
        snapshot_path = workdir / "snapshot.json"
        log_path = workdir / "k6.log"

        env = dict(os.environ)
        for name in secret_envs:
            # 敏感值从部署环境注入（脚本以 __ENV.<name> 引用，环境变量同名提供）。
            env[name] = os.environ.get(name, "")

        command = [k6_path, "run", "--out", f"json={ndjson_path}", "script.js"]
        creationflags = getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
        log_handle = open(log_path, "w", encoding="utf-8")
        try:
            process = subprocess.Popen(
                command,
                cwd=str(workdir),
                env=env,
                stdout=log_handle,
                stderr=subprocess.STDOUT,
                **hidden_subprocess_kwargs(creationflags=creationflags),
            )
        except OSError as error:
            log_handle.close()
            return self._result(scenario_type, exit_code=1, failure_stage="k6_runtime", error=f"启动 k6 失败：{error}", partial=False, terminal_status="execution_failed")

        if started:
            try:
                started({
                    "script_hash": script_hash,
                    "generator_version": GENERATOR_VERSION,
                    "k6_version": k6_version,
                })
            except Exception:
                logger.exception("性能测试 running 回调失败")

        sampler = NdjsonSampler(
            ndjson_path,
            snapshot_path,
            task_id=task_id,
            scenario_type=scenario_type,
            load_config=load_config,
            thresholds=thresholds,
            sample_interval_ms=sample_interval_ms,
            sample_callback=sample,
        )
        sampler.start()

        started_at = time.monotonic()
        while True:
            if canceled and canceled():
                return self._handle_cancel(process, sampler, report_path, snapshot_path, log_path, log_handle, scenario_type, progress)
            try:
                process.wait(timeout=0.2)
                break
            except subprocess.TimeoutExpired:
                pass
            if time.monotonic() - started_at > timeout:
                self._force_kill(process)
                sampler.stop()
                log_handle.close()
                partial = self._load_snapshot(snapshot_path) or sampler.snapshot()
                diagnostic = self._read_diagnostic(log_path)
                error = "性能测试超时"
                if progress:
                    progress(f"[超时] {error}")
                return self._result(scenario_type, exit_code=1, failure_stage="timeout", error=error, metrics=partial, summary=_summary_from_metrics(partial), series=sampler.series(True), diagnostic=diagnostic, partial=True, terminal_status="timed_out")

        returncode = process.returncode
        sampler.stop()
        log_handle.close()
        diagnostic = self._read_diagnostic(log_path)

        report = None
        report_error = None
        if report_path.exists():
            try:
                report = json.loads(report_path.read_text(encoding="utf-8"))
            except (json.JSONDecodeError, OSError) as error:
                report_error = error

        if isinstance(report, dict):
            thresholds_ok, threshold_details = self._evaluate_thresholds(report)
            metrics = self._extract_metrics(report)
            if not thresholds_ok:
                # k6 threshold 失败可能以非零退出码结束，必须优先使用 report 判定。
                elapsed_seconds = sampler.elapsed_seconds() if hasattr(sampler, "elapsed_seconds") else 0.0
                total_duration = sampler.duration_seconds() if hasattr(sampler, "duration_seconds") else 0.0
                partial = self._threshold_abort_early(thresholds or [], threshold_details, elapsed_seconds, total_duration)
                return self._result(
                    scenario_type,
                    exit_code=returncode,
                    failure_stage="",
                    error="",
                    terminal_status="threshold_failed",
                    thresholds_ok=False,
                    thresholds=threshold_details,
                    metrics=metrics,
                    summary=report,
                    series=sampler.series(partial),
                    diagnostic=diagnostic,
                    partial=partial,
                )
            if returncode != 0:
                error = f"k6 执行失败（exit_code={returncode}）"
                if progress:
                    progress(f"[失败] {error}")
                return self._result(
                    scenario_type,
                    exit_code=returncode,
                    failure_stage="k6_runtime",
                    error=error,
                    thresholds_ok=True,
                    thresholds=threshold_details,
                    metrics=metrics,
                    summary=report,
                    series=sampler.series(True),
                    diagnostic=diagnostic,
                    partial=False,
                    terminal_status="execution_failed",
                )
            if progress:
                progress("[完成] k6 压测完成")
            return self._result(
                scenario_type,
                exit_code=0,
                failure_stage="",
                error="",
                terminal_status="completed",
                thresholds_ok=True,
                thresholds=threshold_details,
                metrics=metrics,
                summary=report,
                series=sampler.series(False),
                diagnostic=diagnostic,
                partial=False,
            )

        if report_error:
            error = f"解析 report.json 失败：{report_error}"
        elif returncode == 0:
            error = "k6 正常结束但未生成 report.json"
        else:
            error = f"k6 执行失败（exit_code={returncode}）"
        return self._result(
            scenario_type,
            exit_code=returncode or 1,
            failure_stage="k6_runtime",
            error=error,
            diagnostic=diagnostic,
            series=sampler.series(True),
            partial=True,
            terminal_status="execution_failed",
        )

    @staticmethod
    def _threshold_abort_early(thresholds: list, details: list[dict], elapsed_seconds: float, total_duration: float) -> bool:
        if not thresholds or not details:
            return False
        failed = {str(item.get("threshold") or "") for item in details if item.get("ok") is False}
        if not failed:
            return False
        try:
            # threshold abort is partial only when an aborting threshold stops the run before its configured duration.
            # The duration is supplied by the sampler through its elapsed comparison below.
            for item in thresholds:
                expression = f"{item.get('aggregation', 'value')}{item.get('operator', '')}{item.get('value', '')}"
                if item.get("abortOnFail") and expression in failed:
                    return elapsed_seconds < total_duration - 0.1
        except (TypeError, ValueError):
            return False
        return False

    def _handle_cancel(self, process, sampler, report_path, snapshot_path, log_path, log_handle, scenario_type, progress) -> TaskResult:
        """优雅停止 → 宽限期 → 强制终止，并用本地采样快照兜底部分结果（SPEC §5.1）。"""
        self._request_stop(process)
        deadline = time.monotonic() + settings.perf_grace_period_seconds
        while time.monotonic() < deadline:
            try:
                process.wait(timeout=0.2)
                break
            except subprocess.TimeoutExpired:
                pass
        else:
            self._force_kill(process)
        sampler.stop()
        log_handle.close()
        diagnostic = self._read_diagnostic(log_path)

        # 优先读 handleSummary 落盘的 report.json，其次用本地采样快照兜底。
        metrics = None
        report = None
        thresholds_ok = None
        threshold_details = []
        if report_path.exists():
            try:
                report = json.loads(report_path.read_text(encoding="utf-8"))
                thresholds_ok, threshold_details = self._evaluate_thresholds(report)
                metrics = self._extract_metrics(report)
            except (json.JSONDecodeError, OSError):
                report = None
        if metrics is None:
            metrics = self._load_snapshot(snapshot_path) or sampler.snapshot()

        if progress:
            progress("[取消] 压测已取消，返回部分结果")
        return self._result(
            scenario_type,
            exit_code=1,
            failure_stage="cancel",
            error="任务已取消",
            thresholds_ok=thresholds_ok,
            thresholds=threshold_details,
            metrics=metrics,
            summary=report if isinstance(report, dict) else _summary_from_metrics(metrics),
            series=sampler.series(True),
            diagnostic=diagnostic,
            partial=True,
            terminal_status="canceled",
        )

    @staticmethod
    def _request_stop(process) -> None:
        try:
            if os.name == "nt":
                process.send_signal(signal.CTRL_BREAK_EVENT)
            else:
                process.send_signal(signal.SIGINT)
        except (ValueError, OSError):
            process.terminate()

    @staticmethod
    def _force_kill(process) -> None:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()

    @staticmethod
    def _load_snapshot(snapshot_path: Path) -> dict | None:
        try:
            return json.loads(snapshot_path.read_text(encoding="utf-8"))
        except (FileNotFoundError, json.JSONDecodeError, OSError):
            return None

    @staticmethod
    def _attach_smoke_response(result: TaskResult, response_path: Path, log_path: Path | None = None) -> TaskResult:
        try:
            response = json.loads(response_path.read_text(encoding="utf-8"))
        except (FileNotFoundError, OSError, json.JSONDecodeError):
            response = None
        if _normalize_smoke_response(response) is None and log_path is not None:
            response = _read_smoke_response_marker(log_path)
        normalized = _normalize_smoke_response(response)
        if normalized is None:
            return result
        try:
            output = json.loads(result.output or "{}")
        except (TypeError, json.JSONDecodeError):
            return result
        if not isinstance(output, dict):
            return result
        output.update(normalized)
        result.output = json.dumps(output, ensure_ascii=False)
        return result

    @staticmethod
    def _read_diagnostic(log_path: Path) -> str:
        try:
            raw = log_path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            return ""
        raw = "\n".join(line for line in raw.splitlines() if _SMOKE_RESPONSE_MARKER not in line)
        # 脱敏后仅保留最后 32 KB（SPEC §3.2）。
        return raw[-32 * 1024:]

    @staticmethod
    def _extract_metrics(report: dict) -> dict:
        metrics = report.get("metrics") or {}
        http_reqs = (metrics.get("http_reqs") or {}).get("values") or {}
        http_req_duration = (metrics.get("http_req_duration") or {}).get("values") or {}
        http_req_failed = (metrics.get("http_req_failed") or {}).get("values") or {}
        return {
            "total_requests": int(http_reqs.get("count") or 0),
            "avg_duration_ms": _round_ms(http_req_duration.get("avg")),
            "p95_duration_ms": _round_ms(http_req_duration.get("p(95)")),
            "p99_duration_ms": _round_ms(http_req_duration.get("p(99)")),
            "error_rate": http_req_failed.get("rate"),
            "rps": http_reqs.get("rate"),
        }

    @staticmethod
    def _evaluate_thresholds(report: dict) -> tuple[bool, list[dict]]:
        details: list[dict] = []
        all_ok = True
        for metric_name, metric in (report.get("metrics") or {}).items():
            for expression, info in (metric.get("thresholds") or {}).items():
                ok = bool(info.get("ok")) if isinstance(info, dict) else False
                if not ok:
                    all_ok = False
                details.append({"metric": metric_name, "threshold": expression, "ok": ok})
        return all_ok, details

    @staticmethod
    def _result(
        scenario_type,
        exit_code,
        failure_stage,
        error,
        *,
        terminal_status="execution_failed",
        thresholds_ok=None,
        thresholds=None,
        metrics=None,
        summary=None,
        series=None,
        diagnostic="",
        partial=False,
    ) -> TaskResult:
        output = {
            "scenario_type": scenario_type,
            "generator_version": GENERATOR_VERSION,
            "terminal_status": terminal_status,
            "failure_stage": failure_stage,
            "thresholds_ok": thresholds_ok,
            "thresholds": thresholds or [],
            "metrics": metrics or {},
            "summary": _sanitize_json_value(summary or {}),
            "series": _sanitize_json_value(series or {"version": 1, "points": [], "statusCodes": {}, "errorTopN": [], "thresholds": []}),
            "partial": partial,
            "diagnostic": _sanitize_text(diagnostic),
        }
        return TaskResult(exitCode=exit_code, output=json.dumps(output, ensure_ascii=False), error=_sanitize_text(error))

    @staticmethod
    def _attach_runtime_metadata(result: TaskResult, script_hash: str, k6_version: str) -> TaskResult:
        try:
            output = json.loads(result.output or "{}")
        except (TypeError, json.JSONDecodeError):
            return result
        if not isinstance(output, dict):
            return result
        output["script_hash"] = script_hash
        output["k6_version"] = k6_version
        result.output = json.dumps(output, ensure_ascii=False)
        return result

    @staticmethod
    def _k6_version(k6_path: str) -> str:
        try:
            version = subprocess.run([k6_path, "version"], capture_output=True, text=True, timeout=5, **hidden_subprocess_kwargs())
            output = version.stdout or version.stderr
            return output.splitlines()[0].strip() if output else ""
        except (OSError, subprocess.SubprocessError):
            return ""


def _coerce_int(value) -> int | None:
    if isinstance(value, bool) or value is None:
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def _coerce_number(value) -> float | int | None:
    if isinstance(value, bool) or value is None:
        return None
    try:
        number = float(value)
    except (TypeError, ValueError):
        return None
    return int(number) if number.is_integer() else number


def _response_header(headers: dict, name: str):
    for key, value in headers.items():
        if str(key).lower() == name.lower():
            return value
    return ""


def _is_sensitive_response_header(name: str) -> bool:
    normalized = str(name or "").lower().replace("_", "-")
    return normalized in {"set-cookie", "cookie", "authorization", "proxy-authorization", "x-api-key"} or bool(_SENSITIVE_KEY_PATTERN.search(normalized))


def _sanitize_smoke_headers(headers) -> dict:
    if not isinstance(headers, dict):
        return {}
    result = {}
    for key, value in headers.items():
        key_text = str(key)
        if _is_sensitive_response_header(key_text):
            result[key_text] = "******"
        elif value is None or isinstance(value, (str, int, float, bool)):
            result[key_text] = value
        else:
            result[key_text] = str(value)
    return result


def _is_binary_content_type(content_type: str) -> bool:
    return bool(_BINARY_CONTENT_TYPE_PATTERN.search(str(content_type or "")))


def _read_smoke_response_marker(log_path: Path) -> dict | None:
    try:
        lines = log_path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return None
    for line in reversed(lines):
        marker_index = line.find(_SMOKE_RESPONSE_MARKER)
        if marker_index < 0:
            continue
        candidates = [line[marker_index + len(_SMOKE_RESPONSE_MARKER):]]
        message_start = line.rfind('msg="', 0, marker_index + 1)
        message_end = line.find('" source=', message_start + 5) if message_start >= 0 else -1
        if message_start >= 0 and message_end > message_start:
            try:
                message = json.loads(line[message_start + 4:message_end + 1])
                if isinstance(message, str) and message.startswith(_SMOKE_RESPONSE_MARKER):
                    candidates.insert(0, message[len(_SMOKE_RESPONSE_MARKER):])
            except json.JSONDecodeError:
                pass
        for candidate in candidates:
            try:
                value, _ = json.JSONDecoder().raw_decode(candidate)
            except json.JSONDecodeError:
                continue
            if isinstance(value, dict):
                return value
    return None


def _normalize_smoke_response(response) -> dict | None:
    if not isinstance(response, dict):
        return None
    status_code = _coerce_int(response.get("statusCode"))
    if status_code is None:
        return None
    headers = _sanitize_smoke_headers(response.get("headers"))
    content_type = str(response.get("contentType") or _response_header(headers, "content-type") or "")
    body = response.get("body")
    if body is None:
        body = ""
    elif not isinstance(body, str):
        body = json.dumps(body, ensure_ascii=False) if isinstance(body, (dict, list)) else str(body)
    encoded_body = body.encode("utf-8", errors="replace")
    reported_size = _coerce_int(response.get("bodySize"))
    body_size = max(len(encoded_body), reported_size or 0)
    is_binary = bool(response.get("isBinary")) or _is_binary_content_type(content_type) or b"\x00" in encoded_body
    if is_binary:
        output_body = ""
        truncated = bool(response.get("truncated"))
    else:
        truncated = bool(response.get("truncated")) or len(encoded_body) > _SMOKE_BODY_LIMIT
        output_body = encoded_body[:_SMOKE_BODY_LIMIT].decode("utf-8", errors="ignore") if truncated else body
    return {
        "statusCode": status_code,
        "headers": headers,
        "body": output_body,
        "durationMs": _coerce_number(response.get("durationMs")) or 0,
        "truncated": truncated,
        "bodySize": body_size,
        "isBinary": is_binary,
        "contentType": content_type,
    }


def _round_ms(value) -> float | None:
    if value is None:
        return None
    return round(float(value))


def _int_value(value) -> int:
    try:
        return int(value or 0)
    except (TypeError, ValueError):
        return 0


def _threshold_passed(actual, operator, target) -> bool:
    if actual is None or target is None:
        return False
    try:
        actual = float(actual)
        target = float(target)
    except (TypeError, ValueError):
        return False
    return {"<": actual < target, "<=": actual <= target, ">": actual > target, ">=": actual >= target}.get(str(operator), False)


def _summary_from_metrics(metrics: dict) -> dict:
    """把无 report 的部分结果包装成最小 k6 summary 结构。"""
    return {
        "metrics": {
            "http_reqs": {"values": {"count": metrics.get("total_requests", 0), "rate": metrics.get("rps")}},
            "http_req_duration": {
                "values": {
                    "avg": metrics.get("avg_duration_ms"),
                    "p(95)": metrics.get("p95_duration_ms"),
                    "p(99)": (metrics.get("window") or {}).get("p99_duration_ms"),
                }
            },
            "http_req_failed": {"values": {"rate": metrics.get("error_rate")}},
        },
        "options": {},
        "state": {},
    }


def _sanitize_text(value: str) -> str:
    text = str(value or "")
    text = _SENSITIVE_QUERY_PATTERN.sub(r"\1***", text)
    return _SENSITIVE_TEXT_PATTERN.sub(r"\1***", text)


def _sanitize_json_value(value):
    if isinstance(value, dict):
        sanitized = {}
        for key, item in value.items():
            key_text = str(key)
            sanitized[key_text] = "***" if _SENSITIVE_KEY_PATTERN.search(key_text) else _sanitize_json_value(item)
        return sanitized
    if isinstance(value, list):
        return [_sanitize_json_value(item) for item in value]
    if isinstance(value, str):
        return _sanitize_text(value)
    return value
