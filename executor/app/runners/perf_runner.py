import json
from collections import deque
import hashlib
import logging
import os
import re
import shutil
import signal
import subprocess
import tempfile
import threading
import time
from pathlib import Path
from typing import Callable

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


logger = logging.getLogger(__name__)

# 脚本生成器版本，随启动回调上报为 generator_version（SPEC §3.2）。
GENERATOR_VERSION = "perf-1.0.0"

# k6 duration 字符串解析（1m / 30s / 1h / 10m30s 等）。
_DURATION_PATTERN = re.compile(r"(\d+(?:\.\d+)?)(ms|s|m|h)")

# 阈值运算符白名单，防注入（SPEC §6）。
_ALLOWED_THRESHOLD_OPERATORS = {"<", ">", "<=", ">=", "==", "!="}

# k6 summaryTrendStats 固定集合（SPEC §4.3）。
_SUMMARY_TREND_STATS = ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"]

# 敏感变量引用 {{secret.xxx}}，渲染为 __ENV.XXX 运行时注入（SPEC §3.4）。
_SECRET_PATTERN = re.compile(r"\{\{\s*secret\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}")
_SENSITIVE_KEY_PATTERN = re.compile(r"(?i)(authorization|cookie|password|passwd|token|secret|api[_-]?key)")
_SENSITIVE_TEXT_PATTERN = re.compile(r"(?i)(authorization|cookie|password|passwd|token|secret|api[_-]?key)(\s*[=:]\s*)([^\s,;]+)")
_SENSITIVE_QUERY_PATTERN = re.compile(r"(?i)([?&](?:authorization|cookie|password|passwd|token|secret|api[_-]?key)=)[^&#\s]+")


def parse_duration(value: str) -> float:
    """把 k6 duration 字符串解析为秒数。"""
    text = str(value or "").strip()
    total = 0.0
    for number, unit in _DURATION_PATTERN.findall(text):
        amount = float(number)
        total += amount * {"ms": 0.001, "s": 1, "m": 60, "h": 3600}[unit]
    if total <= 0:
        raise ValueError(f"无效的时长：{value}")
    return total


def total_duration_seconds(scenario_type: str, load_config: dict) -> float:
    """按场景类型计算负载总时长（秒），用于超时动态计算（SPEC §5.1）。"""
    scenario_type = str(scenario_type or "baseline").lower()
    load_config = load_config or {}
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
        raise ValueError("mixed 场景 P0 阶段暂不支持")
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
    if scenario_type in ("baseline", "soak"):
        return {
            "vus": int(load_config.get("vus") or 1),
            "duration": str(load_config.get("duration") or "1m"),
        }
    if scenario_type == "ramp":
        stages = load_config.get("stages") or []
        if not stages:
            raise ValueError("ramp 场景必须提供 load_config.stages")
        return {
            "executor": "ramping-vus",
            "stages": [
                {"duration": str(stage.get("duration") or "0s"), "target": int(stage.get("target") or 0)}
                for stage in stages
            ],
        }
    if scenario_type == "peak":
        peak_vus = int(load_config.get("peakVus") or 0)
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
        step_duration = str(load_config.get("stepDuration") or "1m")
        stages = []
        current = start_vus
        stages.append({"duration": step_duration, "target": current})
        while current < max_vus:
            current = min(max_vus, current + step_vus)
            stages.append({"duration": step_duration, "target": current})
        return {"executor": "ramping-vus", "stages": stages}
    if scenario_type == "mixed":
        raise ValueError("mixed 场景 P0 阶段暂不支持")
    raise ValueError(f"不支持的场景类型：{scenario_type}")


def render_thresholds(thresholds: list) -> dict[str, list[str]]:
    """把平台结构化阈值数组渲染为 k6 thresholds 映射（SPEC §3.1）。

    形如 [{"metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 500}]
    渲染为 {"http_req_duration": ["p(95)<500"]}。
    """
    result: dict[str, list[str]] = {}
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
        result.setdefault(metric, []).append(f"{aggregation}{operator}{value}")
    return result


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
        if isinstance(value, str):
            value_js = _render_js_string(value, secret_envs)
        else:
            value_js = json.dumps(value, ensure_ascii=False)
        pairs.append(f"{key_js}: {value_js}")
    return "{" + ", ".join(pairs) + "}"


def _render_body(body, secret_envs: dict[str, str]) -> str:
    if body is None:
        return "null"
    if isinstance(body, str):
        return _render_js_string(body, secret_envs)
    return json.dumps(body, ensure_ascii=False)


def generate_script(payload: dict) -> tuple[str, dict[str, str]]:
    """渲染 k6 脚本，返回 (脚本文本, 敏感环境变量映射)。"""
    scenario_type = str(payload.get("scenario_type") or "baseline").lower()
    load_config = payload.get("load_config") or {}

    options = render_options(scenario_type, load_config)
    thresholds = render_thresholds(payload.get("thresholds") or [])
    options["summaryTrendStats"] = _SUMMARY_TREND_STATS
    if thresholds:
        options["thresholds"] = thresholds

    target = str(payload.get("target") or payload.get("url") or "").strip()
    if not target:
        raise ValueError("perf 任务必须提供 payload.target")
    method = str(payload.get("method") or "GET").upper()

    secret_envs: dict[str, str] = {}
    target_js = _render_js_string(target, secret_envs)
    headers_js = _render_headers(payload.get("headers") or {}, secret_envs)
    body_js = _render_body(payload.get("body"), secret_envs)

    options_js = json.dumps(options, ensure_ascii=False)
    script = (
        "// 由 Synapse QA 执行器生成，generator_version=" + GENERATOR_VERSION + "\n"
        "import http from 'k6/http';\n"
        "\n"
        "export const options = " + options_js + ";\n"
        "\n"
        "const target = " + target_js + ";\n"
        "const method = " + json.dumps(method, ensure_ascii=False) + ";\n"
        "const headers = " + headers_js + ";\n"
        "const body = " + body_js + ";\n"
        "\n"
        "export default function () {\n"
        "  http.request(method, target, body, { headers: headers });\n"
        "}\n"
        "\n"
        "export function handleSummary(data) {\n"
        "  return {\n"
        "    'report.json': JSON.stringify(data),\n"
        "  };\n"
        "}\n"
    )
    return script, secret_envs


class NdjsonSampler:
    """消费 k6 `--out json` 的 NDJSON，聚合为短周期采样并定期原子写快照（SPEC §7.1）。"""

    def __init__(self, ndjson_path: Path, snapshot_path: Path) -> None:
        self._ndjson_path = ndjson_path
        self._snapshot_path = snapshot_path
        self._interval = settings.perf_snapshot_interval_seconds
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._lock = threading.Lock()
        self._count = 0
        self._durations = deque(maxlen=settings.perf_ndjson_window_points)
        self._recent_requests = deque(maxlen=settings.perf_ndjson_window_points)
        self._recent_failures = deque(maxlen=settings.perf_ndjson_window_points)
        self._fails = 0
        self._total = 0
        self._started_at = time.monotonic()

    def start(self) -> None:
        self._thread = threading.Thread(target=self._run, name="perf-ndjson-sampler", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=2)

    def snapshot(self) -> dict:
        with self._lock:
            return self._aggregate()

    def _run(self) -> None:
        offset = 0
        last_flush = time.monotonic()
        while not self._stop.is_set():
            try:
                with open(self._ndjson_path, "r", encoding="utf-8") as handle:
                    handle.seek(offset)
                    for line in handle:
                        self._consume_line(line)
                    offset = handle.tell()
            except (FileNotFoundError, OSError):
                pass
            if time.monotonic() - last_flush >= self._interval:
                self._write_snapshot()
                last_flush = time.monotonic()
            time.sleep(0.5)
        self._write_snapshot()

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
            if metric == "http_reqs":
                # k6 --out json 每个 http_reqs Point 代表一次计数，应累计而非覆盖。
                count = int(value)
                self._count += count
                self._recent_requests.append((time.monotonic(), count))
            elif metric == "http_req_duration":
                self._durations.append(float(value))
            elif metric == "http_req_failed":
                self._total += 1
                failed = bool(value)
                self._recent_failures.append(failed)
                if failed:
                    self._fails += 1

    def _aggregate(self) -> dict:
        durations = sorted(self._durations)
        now = time.monotonic()
        elapsed = max(0.001, now - self._started_at)
        window_elapsed = elapsed
        if self._recent_requests:
            window_elapsed = max(0.001, now - self._recent_requests[0][0])
        window_count = sum(count for _, count in self._recent_requests)
        window_error_rate = (
            sum(1 for failed in self._recent_failures if failed) / len(self._recent_failures)
            if self._recent_failures
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
        }
        return {
            "total_requests": self._count,
            "avg_duration_ms": round(sum(durations) / len(durations)) if durations else None,
            "p95_duration_ms": self._percentile(durations, 0.95),
            "error_rate": round(self._fails / self._total, 4) if self._total else None,
            "rps": round(self._count / elapsed, 2),
            "window": window,
            "partial": True,
        }

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
    ) -> TaskResult:
        payload = task.payload or {}
        scenario_type = str(payload.get("scenario_type") or "baseline").lower()

        if progress:
            progress("[检查] 正在检测 k6 运行环境")
        k6_path = shutil.which("k6")
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
            )
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
        script_hash: str,
        k6_version: str,
        started: Callable[[dict], None] | None,
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
        creationflags = subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0
        log_handle = open(log_path, "w", encoding="utf-8")
        try:
            process = subprocess.Popen(
                command,
                cwd=str(workdir),
                env=env,
                stdout=log_handle,
                stderr=subprocess.STDOUT,
                creationflags=creationflags,
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

        sampler = NdjsonSampler(ndjson_path, snapshot_path)
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
                return self._result(scenario_type, exit_code=1, failure_stage="timeout", error=error, metrics=partial, summary=_summary_from_metrics(partial), diagnostic=diagnostic, partial=True, terminal_status="timed_out")

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
                    diagnostic=diagnostic,
                    partial=False,
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
            partial=True,
            terminal_status="execution_failed",
        )

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
    def _read_diagnostic(log_path: Path) -> str:
        try:
            raw = log_path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            return ""
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
            version = subprocess.run([k6_path, "version"], capture_output=True, text=True, timeout=5)
            output = version.stdout or version.stderr
            return output.splitlines()[0].strip() if output else ""
        except (OSError, subprocess.SubprocessError):
            return ""


def _round_ms(value) -> float | None:
    if value is None:
        return None
    return round(float(value))


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
    return _SENSITIVE_TEXT_PATTERN.sub(r"\1\2***", text)


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
