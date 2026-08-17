import json
import urllib.error
from datetime import datetime, timedelta
from email.message import Message
from unittest.mock import Mock, patch

import pytest

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult, TaskStatus, TaskType, TaskView
from app.runners.perf_runner import (
    PerfRunner,
    compute_timeout,
    generate_script,
    parse_duration,
    render_options,
    render_thresholds,
    total_duration_seconds,
)
from app.services.callback_client import notify_callback
from app.services.task_manager import TaskManager


# ---------------------------------------------------------------------------
# 脚本生成：各场景 options 与阈值渲染
# ---------------------------------------------------------------------------


def test_render_options_baseline_and_soak():
    assert render_options("baseline", {"vus": 3, "duration": "2m"}) == {"vus": 3, "duration": "2m"}
    assert render_options("soak", {"vus": 100, "duration": "30m"}) == {"vus": 100, "duration": "30m"}


def test_render_options_ramp_uses_ramping_vus():
    options = render_options("ramp", {"stages": [{"duration": "1m", "target": 10}, {"duration": "1m", "target": 50}]})
    assert options["executor"] == "ramping-vus"
    assert options["stages"] == [{"duration": "1m", "target": 10}, {"duration": "1m", "target": 50}]


def test_render_options_peak_generates_ramp_hold_rampdown():
    options = render_options("peak", {"peakVus": 150, "rampDuration": "2m", "holdDuration": "30m", "rampDownDuration": "2m"})
    assert options["executor"] == "ramping-vus"
    assert options["stages"] == [
        {"duration": "2m", "target": 150},
        {"duration": "30m", "target": 150},
        {"duration": "2m", "target": 0},
    ]


def test_render_options_stress_increments_to_max():
    options = render_options("stress", {"startVus": 10, "stepVus": 10, "stepDuration": "1m", "maxVus": 30})
    assert options["executor"] == "ramping-vus"
    assert [stage["target"] for stage in options["stages"]] == [10, 20, 30]


def test_render_options_mixed_not_supported():
    with pytest.raises(ValueError):
        render_options("mixed", {})


def test_render_thresholds():
    thresholds = [
        {"metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 500, "unit": "ms"},
        {"metric": "http_req_failed", "aggregation": "rate", "operator": "<", "value": 0.01},
    ]
    assert render_thresholds(thresholds) == {
        "http_req_duration": ["p(95)<500"],
        "http_req_failed": ["rate<0.01"],
    }


def test_render_thresholds_ignores_bad_operator_and_missing_value():
    thresholds = [
        {"metric": "http_req_duration", "aggregation": "p(95)", "operator": "DROP", "value": 500},
        {"metric": "http_req_failed", "aggregation": "rate", "operator": "<"},
    ]
    assert render_thresholds(thresholds) == {}


def test_generate_script_contains_options_and_handle_summary():
    script, secret_envs = generate_script({
        "scenario_type": "baseline",
        "load_config": {"vus": 3, "duration": "2m"},
        "target": "http://example.com/api",
        "method": "GET",
        "thresholds": [{"metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 500}],
    })
    assert secret_envs == {}
    assert "import http from 'k6/http'" in script
    assert "export const options" in script
    assert "summaryTrendStats" in script
    assert "p(95)<500" in script
    assert "handleSummary" in script
    assert "'report.json'" in script


def test_generate_script_escapes_headers_and_body():
    script, _ = generate_script({
        "scenario_type": "baseline",
        "load_config": {"vus": 1, "duration": "1m"},
        "target": "http://example.com",
        "method": "POST",
        "headers": {"X-Custom": 'a"b\nc'},
        "body": '{"key": "value"}',
    })
    # headers 中的双引号/换行必须被 JSON 转义，避免 JS 注入/语法错误。
    assert json.dumps('a"b\nc') in script
    assert json.dumps('{"key": "value"}') in script


def test_generate_script_injects_secret_as_env_reference():
    script, secret_envs = generate_script({
        "scenario_type": "baseline",
        "load_config": {"vus": 1, "duration": "1m"},
        "target": "http://example.com",
        "headers": {"Authorization": "{{secret.api_token}}"},
    })
    assert secret_envs == {"api_token": "api_token"}
    assert "__ENV.api_token" in script
    # 明文敏感值不得出现在脚本中。
    assert "secret.api_token" not in script.replace("__ENV.api_token", "")


# ---------------------------------------------------------------------------
# 超时动态计算
# ---------------------------------------------------------------------------


def test_parse_duration():
    assert parse_duration("2m") == 120
    assert parse_duration("30s") == 30
    assert parse_duration("1h") == 3600
    assert parse_duration("10m30s") == 630


def test_total_duration_baseline_soak_ramp():
    assert total_duration_seconds("baseline", {"duration": "2m"}) == 120
    assert total_duration_seconds("soak", {"duration": "30m"}) == 1800
    assert total_duration_seconds("ramp", {"stages": [{"duration": "1m", "target": 10}, {"duration": "2m", "target": 50}]}) == 180


def test_total_duration_peak_and_stress():
    peak = total_duration_seconds("peak", {"peakVus": 150, "rampDuration": "2m", "holdDuration": "30m", "rampDownDuration": "2m"})
    assert peak == 2040
    stress = total_duration_seconds("stress", {"startVus": 10, "stepVus": 10, "stepDuration": "1m", "maxVus": 30})
    assert stress == 180


def test_compute_timeout_is_dynamic_not_fixed(monkeypatch):
    monkeypatch.setattr(settings, "perf_timeout_coefficient", 1.0)
    monkeypatch.setattr(settings, "perf_timeout_buffer_seconds", 0)
    # 长时场景超时上限远大于固定 300s。
    assert compute_timeout("soak", {"duration": "30m"}) == 1800
    assert compute_timeout("baseline", {"duration": "2m"}) == 120


# ---------------------------------------------------------------------------
# k6 缺失检测
# ---------------------------------------------------------------------------


def test_perf_runner_reports_missing_k6(monkeypatch):
    monkeypatch.setattr("app.runners.perf_runner.shutil.which", lambda name: None)
    runner = PerfRunner()
    task = TaskCreate(taskId="perf-1", type=TaskType.perf, payload={"scenario_type": "baseline", "target": "http://example.com"})
    result = runner.run(task)
    assert result.exit_code == 1
    assert "未安装 k6" in result.error
    output = json.loads(result.output)
    assert output["failure_stage"] == "startup"


# ---------------------------------------------------------------------------
# submit 幂等与取消墓碑
# ---------------------------------------------------------------------------


def test_submit_same_task_id_is_idempotent():
    manager = TaskManager()
    manager._runners[TaskType.noop] = Mock(return_value=Mock(exit_code=0, output=""))
    first = manager.submit(TaskCreate(taskId="idem-1", type=TaskType.noop, payload={}))
    second = manager.submit(TaskCreate(taskId="idem-1", type=TaskType.noop, payload={}))
    assert second.task_id == first.task_id


def test_cancel_nonexistent_records_tombstone_and_returns_canceled():
    manager = TaskManager()
    view = manager.cancel("ghost-task")
    assert view.status.value == "canceled"
    # 命中墓碑后提交应幂等返回已取消。
    submitted = manager.submit(TaskCreate(taskId="ghost-task", type=TaskType.perf, payload={}))
    assert submitted.status.value == "canceled"


def test_submit_after_cancel_returns_canceled():
    manager = TaskManager()
    manager._runners[TaskType.noop] = Mock(return_value=Mock(exit_code=0, output=""))
    manager.submit(TaskCreate(taskId="idem-2", type=TaskType.noop, payload={}))
    manager.cancel("idem-2")
    submitted = manager.submit(TaskCreate(taskId="idem-2", type=TaskType.noop, payload={}))
    assert submitted.status.value == "canceled"


# ---------------------------------------------------------------------------
# callback 重试与停止逻辑
# ---------------------------------------------------------------------------


def _callback_task() -> TaskView:
    return TaskView(
        taskId="cb-1",
        type=TaskType.perf,
        status=TaskStatus.success,
        payload={},
        callbackUrl="http://backend/callback",
        callbackToken="secret-token",
        createdAt=datetime.now(),
    )


def test_callback_sends_bearer_token(monkeypatch):
    monkeypatch.setattr(settings, "callback_max_attempts", 1)
    monkeypatch.setattr("app.services.callback_client.time.sleep", lambda _: None)
    captured = {}

    def fake_urlopen(request, timeout=5):
        captured["headers"] = request.headers
        response = Mock()
        response.close.return_value = None
        return response

    with patch("urllib.request.urlopen", side_effect=fake_urlopen):
        notify_callback(_callback_task())

    assert captured["headers"]["Authorization"] == "Bearer secret-token"


def test_perf_callback_payload_flattens_runner_output(monkeypatch):
    monkeypatch.setattr(settings, "callback_max_attempts", 1)
    started = datetime.now()
    finished = started + timedelta(milliseconds=3456)
    task = TaskView(
        taskId="perf-cb-1",
        type=TaskType.perf,
        status=TaskStatus.success,
        payload={},
        callbackUrl="http://backend/callback",
        callbackToken="secret-token",
        result=TaskResult(
            exitCode=0,
            output=json.dumps({
                "generator_version": "perf-1.0.0",
                "k6_version": "v1.0.0",
                "thresholds_ok": True,
                "thresholds": [{"ok": True}],
                "metrics": {
                    "total_requests": 3571,
                    "avg_duration_ms": 3,
                    "p95_duration_ms": 5,
                    "error_rate": 0,
                    "rps": 100,
                },
                "diagnostic": "ok",
            }),
        ),
        createdAt=started,
        startedAt=started,
        finishedAt=finished,
    )
    captured = {}

    def fake_urlopen(request, timeout=5):
        captured["body"] = json.loads(request.data.decode("utf-8"))
        response = Mock()
        response.close.return_value = None
        return response

    with patch("urllib.request.urlopen", side_effect=fake_urlopen):
        notify_callback(task)

    payload = captured["body"]
    assert payload["status"] == "completed"
    assert payload["totalRequests"] == 3571
    assert payload["p95DurationMs"] == 5
    assert payload["durationMs"] == 3456
    assert payload["generatorVersion"] == "perf-1.0.0"
    assert payload["k6Version"] == "v1.0.0"
    assert payload["summary"]["thresholds"][0]["ok"] is True
    assert payload["diagnosticOutput"] == "ok"
    assert "result" not in payload


def test_perf_callback_maps_threshold_and_running_status(monkeypatch):
    monkeypatch.setattr(settings, "callback_max_attempts", 1)
    statuses = []

    def fake_urlopen(request, timeout=5):
        statuses.append(json.loads(request.data.decode("utf-8"))["status"])
        response = Mock()
        response.close.return_value = None
        return response

    started = datetime.now()
    running = TaskView(
        taskId="perf-running",
        type=TaskType.perf,
        status=TaskStatus.running,
        payload={},
        callbackUrl="http://backend/callback",
        callbackToken="secret-token",
        createdAt=started,
        startedAt=started,
    )
    threshold_failed = TaskView(
        taskId="perf-threshold",
        type=TaskType.perf,
        status=TaskStatus.success,
        payload={},
        callbackUrl="http://backend/callback",
        callbackToken="secret-token",
        result=TaskResult(exitCode=0, output=json.dumps({"thresholds_ok": False})),
        createdAt=started,
        startedAt=started,
        finishedAt=started,
    )
    with patch("urllib.request.urlopen", side_effect=fake_urlopen):
        notify_callback(running)
        notify_callback(threshold_failed)

    assert statuses == ["running", "threshold_failed"]


def test_task_manager_perf_callback_has_running_and_terminal_events(monkeypatch):
    manager = TaskManager()
    runner = Mock()
    runner.run.return_value = TaskResult(
        exitCode=0,
        output=json.dumps({"thresholds_ok": True, "metrics": {"total_requests": 1}}),
    )
    manager._runners[TaskType.perf] = runner
    statuses = []
    monkeypatch.setattr("app.services.task_manager.notify_callback", lambda task: statuses.append(task.status.value))
    view = manager.submit(TaskCreate(taskId="perf-lifecycle", type=TaskType.perf, payload={}, callbackUrl="http://backend/callback"))
    manager._futures[view.task_id].result(timeout=2)
    assert statuses == ["running", "success"]


def test_callback_stops_retry_on_gone(monkeypatch):
    monkeypatch.setattr(settings, "callback_max_attempts", 5)
    monkeypatch.setattr("app.services.callback_client.time.sleep", lambda _: None)
    calls = []
    error = urllib.error.HTTPError("http://backend/callback", 410, "Gone", Message(), None)

    def fake_urlopen(request, timeout=5):
        calls.append(request)
        raise error

    with patch("urllib.request.urlopen", side_effect=fake_urlopen):
        notify_callback(_callback_task())

    # 410 表示后端已终结，立即停止重试，不再请求。
    assert len(calls) == 1


def test_callback_stops_retry_on_conflict(monkeypatch):
    monkeypatch.setattr(settings, "callback_max_attempts", 5)
    monkeypatch.setattr("app.services.callback_client.time.sleep", lambda _: None)
    calls = []
    error = urllib.error.HTTPError("http://backend/callback", 409, "Conflict", Message(), None)

    def fake_urlopen(request, timeout=5):
        calls.append(request)
        raise error

    with patch("urllib.request.urlopen", side_effect=fake_urlopen):
        notify_callback(_callback_task())

    assert len(calls) == 1


def test_callback_retries_on_transient_error(monkeypatch):
    monkeypatch.setattr(settings, "callback_max_attempts", 3)
    monkeypatch.setattr(settings, "callback_retry_base_delay_seconds", 0)
    calls = []

    def fake_urlopen(request, timeout=5):
        calls.append(request)
        raise OSError("连接失败")

    with patch("urllib.request.urlopen", side_effect=fake_urlopen):
        notify_callback(_callback_task())

    assert len(calls) == 3
