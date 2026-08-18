import json
import urllib.error
from datetime import datetime, timedelta
from email.message import Message
from pathlib import Path
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
    NdjsonSampler,
    _downsample_series_points,
)
from app.services.callback_client import notify_callback
from app.services.task_manager import TaskManager


# ---------------------------------------------------------------------------
# 脚本生成：各场景 options 与阈值渲染
# ---------------------------------------------------------------------------


def test_render_options_baseline_and_soak():
    assert render_options("baseline", {"vus": 3, "duration": "2m"}) == {"vus": 3, "duration": "2m"}
    assert render_options("soak", {"vus": 100, "duration": "30m"}) == {"vus": 100, "duration": "30m"}


def test_render_options_smoke_is_single_iteration():
    options = render_options("smoke", {})
    assert options["scenarios"]["smoke"] == {"executor": "shared-iterations", "iterations": 1, "vus": 1}
    assert total_duration_seconds("smoke", {}) == 1
    script, _ = generate_script({"mode": "smoke", "target": "http://127.0.0.1:8080/api/health"})
    assert '"iterations": 1' in script
    assert '"executor": "shared-iterations"' in script
    assert '"duration": "1m"' not in script


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
    assert total_duration_seconds("stress", {"startVus": 10, "stepVus": 10, "stepDuration": "1m", "maxVus": 500}) == 50 * 60


def test_render_options_mixed_uses_weighted_flow():
    options = render_options("mixed", {"vus": 1, "duration": "3s", "thinkTime": "10ms", "scenarios": [{"name": "a", "weight": 1, "method": "GET", "url": "/a"}]})
    assert options["scenarios"]["mixedFlow"]["executor"] == "constant-vus"


def test_render_thresholds():
    thresholds = [
        {"metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 500, "unit": "ms"},
        {"metric": "http_req_failed", "aggregation": "rate", "operator": "<", "value": 0.01},
    ]
    assert render_thresholds(thresholds) == {
        "http_req_duration": [{"threshold": "p(95)<500", "abortOnFail": False}],
        "http_req_failed": [{"threshold": "rate<0.01", "abortOnFail": False}],
    }


def test_render_thresholds_ignores_bad_operator_and_missing_value():
    thresholds = [
        {"metric": "http_req_duration", "aggregation": "p(95)", "operator": "DROP", "value": 500},
        {"metric": "http_req_failed", "aggregation": "rate", "operator": "<"},
    ]
    assert render_thresholds(thresholds) == {}


def test_render_thresholds_only_emits_nonempty_abort_delay():
    result = render_thresholds([
        {"metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 500, "abortOnFail": True, "delayAbortEval": "0.5s"},
        {"metric": "http_reqs", "aggregation": "count", "operator": ">", "value": 0, "abortOnFail": True},
    ])
    assert result["http_req_duration"][0]["delayAbortEval"] == "0.5s"
    assert "delayAbortEval" not in result["http_reqs"][0]


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


def test_generate_script_supports_embedded_secrets_in_target_headers_and_body():
    script, secret_envs = generate_script({
        "scenario_type": "baseline",
        "load_config": {"vus": 1, "duration": "1s"},
        "target": "https://example.com/items?token={{secret.query_token}}",
        "method": "POST",
        "headers": {"Authorization": "Bearer {{secret.api_token}}"},
        "body": '{"password":"{{secret.password}}"}',
    })
    assert secret_envs == {"query_token": "query_token", "api_token": "api_token", "password": "password"}
    assert "__ENV.query_token" in script
    assert "__ENV.api_token" in script
    assert "__ENV.password" in script


def test_generate_script_mixed_contains_weighted_random_flow_and_relative_url():
    script, _ = generate_script({
        "scenario_type": "mixed",
        "load_config": {"vus": 1, "duration": "3s", "thinkTime": "10ms", "scenarios": [
            {"name": "health", "weight": 0.6, "method": "GET", "url": "/health"},
            {"name": "other", "weight": 0.4, "method": "GET", "url": "/other"},
        ]},
        "target": "http://example.com/base",
        "method": "GET",
    })
    assert "mixedFlow" in script
    assert "Math.random" in script
    assert "resolveMixedTarget(selected.url)" in script
    assert "sleep(" in script


def test_mixed_all_absolute_urls_do_not_require_global_target():
    script, _ = generate_script({
        "scenario_type": "mixed",
        "load_config": {"vus": 1, "duration": "1s", "thinkTime": "1ms", "scenarios": [{"name": "a", "weight": 1, "method": "GET", "url": "http://127.0.0.1/a"}]},
        "target": "",
    })
    assert "http://127.0.0.1/a" in script
    with pytest.raises(ValueError):
        generate_script({"scenario_type": "mixed", "load_config": {"vus": 1, "duration": "1s", "thinkTime": "1ms", "scenarios": [{"name": "a", "weight": 1, "method": "GET", "url": "/a"}]}, "target": ""})


def test_vus_hard_limit_at_500():
    assert render_options("baseline", {"vus": 500, "duration": "1s"})["vus"] == 500
    with pytest.raises(ValueError):
        render_options("baseline", {"vus": 501, "duration": "1s"})
    with pytest.raises(ValueError):
        render_options("ramp", {"stages": [{"duration": "1s", "target": 501}]})


def test_generate_script_mixed_parses_think_time_and_nested_secrets():
    script, secret_envs = generate_script({
        "scenario_type": "mixed",
        "load_config": {"vus": 1, "duration": "3s", "thinkTime": "0.5s", "scenarios": [
            {"name": "health", "weight": 1, "method": "POST", "url": "api/{{secret.path}}", "headers": {"Authorization": "Bearer {{secret.token}}"}, "body": {"password": "{{secret.password}}"}},
        ]},
        "target": "http://example.com/api/base",
        "method": "GET",
    })
    assert "sleep(0.5)" in script
    assert "__ENV.path" in script and "__ENV.token" in script and "__ENV.password" in script
    assert secret_envs == {"path": "path", "token": "token", "password": "password"}


def test_mixed_weight_tolerance_is_strict():
    with pytest.raises(ValueError):
        render_options("mixed", {"vus": 1, "duration": "1s", "thinkTime": "1ms", "scenarios": [{"name": "a", "weight": 0.999, "url": "/a"}]})


# ---------------------------------------------------------------------------
# 超时动态计算
# ---------------------------------------------------------------------------


def test_parse_duration():
    assert parse_duration("2m") == 120
    assert parse_duration("30s") == 30
    assert parse_duration("1h") == 3600
    assert parse_duration("10m30s") == 630
    with pytest.raises(ValueError):
        parse_duration("prefix1s")


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
    assert output["terminal_status"] == "execution_failed"


def test_perf_runner_uses_report_threshold_failure_before_nonzero_exit(monkeypatch, tmp_path):
    class FakeProcess:
        returncode = 99

        def wait(self, timeout=None):
            return None

        def poll(self):
            return self.returncode

    class FakeSampler:
        def __init__(self, *_args, **_kwargs):
            pass

        def start(self):
            pass

        def stop(self):
            pass

        def snapshot(self):
            return {}

        def series(self, partial):
            return {"version": 1, "points": [], "statusCodes": {}, "errorTopN": [], "thresholds": [], "partial": partial}

    def fake_popen(_command, cwd, **_kwargs):
        Path(cwd, "report.json").write_text(json.dumps({
            "metrics": {
                "http_req_duration": {
                    "values": {"p(95)": 900},
                    "thresholds": {"p(95)<500": {"ok": False}},
                }
            },
            "options": {},
            "state": {},
        }), encoding="utf-8")
        return FakeProcess()

    monkeypatch.setattr("app.runners.perf_runner.subprocess.Popen", fake_popen)
    monkeypatch.setattr("app.runners.perf_runner.NdjsonSampler", FakeSampler)
    result = PerfRunner()._run_k6("k6", tmp_path, {}, "baseline", 10, None, None, "hash", "v1", None)
    output = json.loads(result.output)
    assert result.exit_code == 99
    assert output["terminal_status"] == "threshold_failed"
    assert output["summary"]["metrics"]["http_req_duration"]["thresholds"]["p(95)<500"]["ok"] is False


def test_ndjson_sampler_accumulates_requests_and_bounds_window(monkeypatch, tmp_path):
    monkeypatch.setattr(settings, "perf_ndjson_window_points", 3)
    sampler = NdjsonSampler(tmp_path / "ndjson.out", tmp_path / "snapshot.json")
    for duration in (10, 20, 30, 40):
        sampler._consume_line(json.dumps({"type": "Point", "metric": "http_reqs", "data": {"value": 1}}))
        sampler._consume_line(json.dumps({"type": "Point", "metric": "http_req_duration", "data": {"value": duration}}))
    snapshot = sampler.snapshot()
    assert snapshot["total_requests"] == 4
    assert snapshot["window"]["sample_points"] == 4
    assert snapshot["window"]["p99_duration_ms"] == 40


def test_ndjson_sampler_evicts_by_timestamp_window(tmp_path):
    now = [100.0]
    sampler = NdjsonSampler(tmp_path / "ndjson.out", tmp_path / "snapshot.json", sample_interval_ms=1000, clock=lambda: now[0])
    sampler._window_seconds = 0.01
    sampler._consume_line(json.dumps({"type": "Point", "metric": "http_req_duration", "data": {"value": 10}}))
    now[0] = 100.02
    snapshot = sampler.snapshot()
    assert snapshot["total_requests"] == 0
    assert snapshot["window"]["sample_points"] == 0


def test_ndjson_sampler_high_throughput_counts_are_not_deque_truncated(tmp_path):
    sampler = NdjsonSampler(tmp_path / "ndjson.out", tmp_path / "snapshot.json", sample_interval_ms=5000)
    for _ in range(15001):
        sampler._consume_line(json.dumps({"type": "Point", "metric": "http_reqs", "data": {"value": 1}}))
        sampler._consume_line(json.dumps({"type": "Point", "metric": "http_req_failed", "data": {"value": 0}}))
        sampler._consume_line(json.dumps({"type": "Point", "metric": "http_req_duration", "data": {"value": 10}}))
    snapshot = sampler.snapshot()
    assert snapshot["total_requests"] == 15001
    assert snapshot["window"]["rps"] > 2500
    assert snapshot["window"]["error_rate"] == 0


def test_sampler_tracks_vus_stage_and_realtime_threshold_aggregation(tmp_path):
    events = []
    sampler = NdjsonSampler(
        tmp_path / "ndjson.out",
        tmp_path / "snapshot.json",
        scenario_type="peak",
        load_config={"peakVus": 4, "rampDuration": "1s", "holdDuration": "2s", "rampDownDuration": "1s"},
        thresholds=[{"metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 50}],
        sample_interval_ms=1000,
        sample_callback=events.append,
    )
    sampler._consume_line(json.dumps({"type": "Point", "metric": "vus", "data": {"value": 4}}))
    for value in (10, 20, 30):
        sampler._consume_line(json.dumps({"type": "Point", "metric": "http_req_duration", "data": {"value": value}}))
    sampler.emit_sample(force=True)
    assert events[0]["vus"] == 4
    assert events[0]["stage"] == "ramp-up"
    assert "thresholds" in events[0]
    assert "thresholds" not in sampler.series(False)["points"][0]
    assert sampler.series(False)["thresholds"][0]["actual"] == 29
    assert sampler.series(False)["thresholds"][0]["status"] == "passing"


def test_sampler_event_contains_distribution_and_threshold_details(tmp_path):
    events = []
    sampler = NdjsonSampler(
        tmp_path / "ndjson.out",
        tmp_path / "snapshot.json",
        thresholds=[{"metric": "http_req_failed", "aggregation": "rate", "operator": "<", "value": 0.1}],
        sample_interval_ms=1000,
        sample_callback=events.append,
    )
    sampler._consume_line(json.dumps({"type": "Point", "metric": "http_reqs", "data": {"value": 1, "tags": {"status": "500"}}}))
    sampler._consume_line(json.dumps({"type": "Point", "metric": "http_req_failed", "data": {"value": 1}}))
    sampler.emit_sample(force=True)
    event = events[0]
    assert event["statusCodes"] == {"500": 1}
    assert event["errorTopN"][0]["count"] == 1
    assert event["thresholds"][0]["status"] == "failing"
    assert event["thresholds"][0]["actual"] == 1
    assert event["message"] == ""


def test_online_series_compression_keeps_edges_time_ranges_and_stage_boundaries(tmp_path):
    sampler = NdjsonSampler(tmp_path / "ndjson.out", tmp_path / "snapshot.json")
    for index in range(7001):
        sampler._append_series_point({"sequence": index, "timestamp": index, "stage": "stage:1" if index < 2000 else "stage:2" if index < 7000 else "stage:tail"})
    points = sampler.series(False)["points"]
    assert len(points) <= 3000
    assert points[0]["sequence"] == 0
    assert points[-1]["sequence"] == 7000
    sequences = {point["sequence"] for point in points}
    assert any(1900 <= value <= 2100 for value in sequences)
    assert 6999 in sequences
    assert 7000 in sequences
    assert any(value < 1000 for value in sequences)
    assert any(3000 <= value <= 4000 for value in sequences)
    assert any(value > 6000 for value in sequences)


def test_downsample_series_points_uniformly_samples_excessive_stage_boundaries():
    points = [{"sequence": index, "stage": f"stage:{index}"} for index in range(7001)]
    sampled = _downsample_series_points(points, limit=31)
    sequences = [point["sequence"] for point in sampled]
    assert len(sampled) == 31
    assert sequences[0] == 0
    assert sequences[-1] == 7000
    assert any(100 <= value <= 300 for value in sequences)
    assert any(2000 <= value <= 3000 for value in sequences)
    assert any(4000 <= value <= 5000 for value in sequences)
    assert any(6000 <= value <= 6900 for value in sequences)
    assert sequences == sorted(sequences)


def test_threshold_failed_partial_only_for_early_abort():
    details = [{"threshold": "p(95)<1", "ok": False}]
    assert PerfRunner._threshold_abort_early([{"aggregation": "p(95)", "operator": "<", "value": 1, "abortOnFail": True}], details, 1, 10)
    assert not PerfRunner._threshold_abort_early([{"aggregation": "p(95)", "operator": "<", "value": 1, "abortOnFail": False}], details, 1, 10)


def test_stress_stage_changes_with_elapsed(tmp_path):
    sampler = NdjsonSampler(tmp_path / "ndjson.out", tmp_path / "snapshot.json", scenario_type="stress", load_config={"startVus": 10, "stepVus": 10, "maxVus": 500, "stepDuration": "1s"})
    assert sampler._stage_for(0) == "stage:1"
    assert sampler._stage_for(1000) == "stage:2"
    assert sampler._stage_for(49_000) == "stage:50"


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
                    "p99_duration_ms": 7,
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
    assert payload["p99DurationMs"] == 7
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


def test_perf_callback_uses_explicit_terminal_status_and_sanitizes_output(monkeypatch):
    monkeypatch.setattr(settings, "callback_max_attempts", 1)
    captured = {}
    task = TaskView(
        taskId="perf-failed",
        type=TaskType.perf,
        status=TaskStatus.failed,
        payload={},
        callbackUrl="http://backend/callback",
        callbackToken="secret-token",
        result=TaskResult(
            exitCode=99,
            error="Authorization: Bearer real-token",
            output=json.dumps({
                "terminal_status": "execution_failed",
                "thresholds_ok": False,
                "metrics": {},
                "summary": {"metrics": {}, "headers": {"Authorization": "real-token"}},
                "diagnostic": "password=real-password",
            }),
        ),
        createdAt=datetime.now(),
    )

    def fake_urlopen(request, timeout=5):
        captured["body"] = json.loads(request.data.decode("utf-8"))
        response = Mock()
        response.close.return_value = None
        return response

    with patch("urllib.request.urlopen", side_effect=fake_urlopen):
        notify_callback(task)
    payload = captured["body"]
    assert payload["status"] == "execution_failed"
    assert payload["summary"]["headers"]["Authorization"] == "***"
    assert "real-password" not in payload["diagnosticOutput"]
    assert "real-token" not in payload["errorMessage"]


def test_task_manager_perf_callback_has_running_and_terminal_events(monkeypatch):
    manager = TaskManager()
    runner = Mock()

    def run(_task, _progress, _canceled, started, _sample):
        started({"script_hash": "abc", "generator_version": "perf-1.0.0", "k6_version": "v1.0.0"})
        return TaskResult(exitCode=0, output=json.dumps({"thresholds_ok": True, "metrics": {"total_requests": 1}}))

    runner.run.side_effect = run
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
