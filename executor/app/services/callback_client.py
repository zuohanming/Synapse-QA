import json
import logging
import re
import time
import urllib.error
import urllib.request

from app.core.config import settings
from app.models.task import TaskStatus, TaskType, TaskView


logger = logging.getLogger(__name__)

# 后端已终结或 Token 失效，收到这些状态码后停止重试（SPEC §4.2/§5.5）。
_STOP_STATUS_CODES = {401, 403, 409, 410}
_SENSITIVE_KEY_PATTERN = re.compile(r"(?i)(authorization|cookie|password|passwd|token|secret|api[_-]?key)")
_SENSITIVE_TEXT_PATTERN = re.compile(r"(?i)(authorization|cookie|password|passwd|token|secret|api[_-]?key)(\s*[=:]\s*)([^\s,;]+)")
_SENSITIVE_QUERY_PATTERN = re.compile(r"(?i)([?&](?:authorization|cookie|password|passwd|token|secret|api[_-]?key)=)[^&#\s]+")


def notify_callback(task: TaskView) -> None:
    """回调任务结果；perf 使用专用扁平协议，其他任务保持通用 TaskView。"""

    if not task.callback_url:
        logger.info("任务无需回调：task_id=%s", task.task_id)
        return
    if task.type == TaskType.perf:
        payload = _perf_callback_payload(task)
    else:
        # callbackToken 仅作请求凭据，不回传（SPEC §4.2 分离 task_id 与回调凭据）。
        payload = task.model_dump(by_alias=True, mode="json", exclude={"callback_token"})
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if task.callback_token:
        headers["Authorization"] = f"Bearer {task.callback_token}"

    for attempt in range(settings.callback_max_attempts):
        request = urllib.request.Request(task.callback_url, data=data, headers=headers, method="POST")
        try:
            urllib.request.urlopen(request, timeout=5).close()
            logger.info("任务回调成功：task_id=%s url=%s", task.task_id, task.callback_url)
            return
        except urllib.error.HTTPError as error:
            status = error.code
            error.close()
            if status in _STOP_STATUS_CODES:
                logger.warning("任务回调被后端拒绝，停止重试：task_id=%s status=%s", task.task_id, status)
                return
            logger.warning("任务回调失败，准备重试：task_id=%s status=%s attempt=%s", task.task_id, status, attempt + 1)
        except Exception as error:
            logger.warning("任务回调失败，准备重试：task_id=%s error=%s attempt=%s", task.task_id, error, attempt + 1)
        if attempt < settings.callback_max_attempts - 1:
            time.sleep(settings.callback_retry_base_delay_seconds * (2 ** attempt))
    logger.error("任务回调最终失败：task_id=%s url=%s", task.task_id, task.callback_url)


def _perf_callback_payload(task: TaskView) -> dict:
    """把 perf runner 的 TaskView/result.output 转为后端 PerfCallbackRequest。"""
    output: dict = {}
    raw_output = task.result.output if task.result else ""
    if raw_output:
        try:
            parsed = json.loads(raw_output)
            if isinstance(parsed, dict):
                output = parsed
        except (TypeError, json.JSONDecodeError):
            # 执行器异常时 output 可能只是进度文本，仍保留到诊断字段。
            output = {}

    metrics_value = output.get("metrics")
    metrics: dict = metrics_value if isinstance(metrics_value, dict) else {}
    failure_stage = str(output.get("failure_stage") or "")
    status = task.status.value if isinstance(task.status, TaskStatus) else str(task.status)
    thresholds_ok = output.get("thresholds_ok")
    terminal_status = output.get("terminal_status")
    if status == TaskStatus.running.value:
        callback_status = "running"
    elif terminal_status in {"completed", "threshold_failed", "execution_failed", "timed_out", "canceled"}:
        callback_status = terminal_status
    else:
        # 兼容旧版执行器产生的历史 TaskView；新 perf 结果始终带 terminal_status。
        if status == TaskStatus.canceled.value or failure_stage == "cancel":
            callback_status = "canceled"
        elif failure_stage in {"timeout", "timed_out"}:
            callback_status = "timed_out"
        elif status == TaskStatus.success.value and thresholds_ok is False:
            callback_status = "threshold_failed"
        elif status == TaskStatus.success.value:
            callback_status = "completed"
        else:
            callback_status = "execution_failed"

    diagnostic = output.get("diagnostic")
    if not isinstance(diagnostic, str):
        diagnostic = raw_output if raw_output and not output else ""
    summary_value = output.get("summary")
    if not isinstance(summary_value, dict) or not isinstance(summary_value.get("metrics"), dict):
        # 历史版本把整个自定义 output 当 summary，保留最小双格式兼容。
        summary_value = output
    duration_ms = _duration_ms(task)
    result = task.result
    return {
        "status": callback_status,
        "scriptHash": _sanitize_text(output.get("script_hash") or ""),
        "generatorVersion": _sanitize_text(output.get("generator_version") or ""),
        "k6Version": _sanitize_text(output.get("k6_version") or ""),
        "exitCode": result.exit_code if result else None,
        "durationMs": duration_ms,
        "totalRequests": metrics.get("total_requests", 0),
        "avgDurationMs": metrics.get("avg_duration_ms"),
        "p95DurationMs": metrics.get("p95_duration_ms"),
        "errorRate": metrics.get("error_rate"),
        "rps": metrics.get("rps"),
        "summary": _sanitize_json_value(summary_value),
        "errorMessage": _sanitize_text(result.error if result else ""),
        "failureStage": _sanitize_text(failure_stage),
        "diagnosticOutput": _sanitize_text(diagnostic),
        "needsAttention": bool(output.get("needs_attention", False)),
    }


def _duration_ms(task: TaskView) -> int | None:
    if not task.started_at or not task.finished_at:
        return None
    return max(0, round((task.finished_at - task.started_at).total_seconds() * 1000))


def _sanitize_text(value: str) -> str:
    text = str(value or "")
    text = _SENSITIVE_QUERY_PATTERN.sub(r"\1***", text)
    return _SENSITIVE_TEXT_PATTERN.sub(r"\1\2***", text)


def _sanitize_json_value(value):
    if isinstance(value, dict):
        return {
            str(key): "***" if _SENSITIVE_KEY_PATTERN.search(str(key)) else _sanitize_json_value(item)
            for key, item in value.items()
        }
    if isinstance(value, list):
        return [_sanitize_json_value(item) for item in value]
    if isinstance(value, str):
        return _sanitize_text(value)
    return value


def notify_event(task: TaskView, sequence: int, event_type: str, stage: str, message: str, progress: int, data: dict | None = None) -> None:
    """实时上报 API 调试步骤；单次失败只记录日志，由最终回调兜底。"""

    if not task.event_url:
        return
    payload = {
        "taskId": task.task_id,
        "sequence": sequence,
        "type": event_type,
        "stage": stage,
        "status": "running",
        "message": message,
        "progress": progress,
        "data": data or {},
    }
    request = urllib.request.Request(
        task.event_url,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        urllib.request.urlopen(request, timeout=3).close()
    except Exception as error:
        logger.warning("任务事件回调失败：task_id=%s type=%s error=%s", task.task_id, event_type, error)
