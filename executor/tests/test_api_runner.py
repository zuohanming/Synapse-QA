import json
import gzip
from unittest.mock import patch

from app.models.task import TaskCreate
from app.runners.api_runner import ApiRunner


class FakeResponse:
    status = 200
    headers = {"Content-Type": "application/json", "X-Trace": "trace-1"}

    def __init__(self, body: bytes, headers: dict | None = None):
        self._body = body
        self._offset = 0
        self.headers = headers or {"Content-Type": "application/json", "X-Trace": "trace-1"}

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return False

    def read(self, size: int) -> bytes:
        chunk = self._body[self._offset:self._offset + size]
        self._offset += len(chunk)
        return chunk


def test_api_runner_extracts_and_runs_all_assertions():
    task = TaskCreate(
        taskId="api-processing",
        type="api",
        payload={
            "url": "https://example.com/users/1",
            "extractors": [
                {"type": "jsonpath", "name": "user_id", "expression": "$.data.id"},
                {"type": "regex", "name": "role", "expression": '"role":"([^"]+)"'},
            ],
            "assertions": [
                {"type": "status", "operator": "equals", "expected": 200},
                {"type": "jsonpath", "expression": "$.data.name", "operator": "equals", "expected": "张三"},
                {"type": "variable", "expression": "role", "operator": "equals", "expected": "admin"},
            ],
        },
    )
    events = []
    body = b'{"data":{"id":7,"name":"\\u5f20\\u4e09"},"role":"admin"}'
    with patch.object(ApiRunner, "_validate_target"), patch("urllib.request.urlopen", return_value=FakeResponse(body)):
        result = ApiRunner().run(task, lambda *args: events.append(args))

    output = json.loads(result.output)
    assert result.exit_code == 0
    assert output["extractedVariables"] == {"user_id": 7, "role": "admin"}
    assert len(output["assertions"]) == 3
    assert all(item["passed"] for item in output["assertions"])
    assert [event[0] for event in events].count("extractor.completed") == 2
    assert [event[0] for event in events].count("assertion.completed") == 3


def test_api_runner_returns_all_assertion_failures():
    task = TaskCreate(
        taskId="api-assert-failed",
        type="api",
        payload={
            "url": "https://example.com",
            "assertions": [
                {"type": "status", "operator": "equals", "expected": 201},
                {"type": "body", "operator": "contains", "expected": "missing"},
            ],
        },
    )
    with patch.object(ApiRunner, "_validate_target"), patch("urllib.request.urlopen", return_value=FakeResponse(b'{"ok":true}')):
        result = ApiRunner().run(task)

    output = json.loads(result.output)
    assert result.exit_code == 1
    assert len(output["assertions"]) == 2
    assert all(not item["passed"] for item in output["assertions"])


def test_api_runner_decompresses_gzip_response_before_display_and_assertion():
    payload = '{"message":"登录成功","user":"张三"}'.encode("utf-8")
    response = FakeResponse(gzip.compress(payload), {
        "Content-Type": "application/json",
        "Content-Encoding": "gzip",
    })
    task = TaskCreate(
        taskId="api-gzip",
        type="api",
        payload={
            "url": "https://example.com/login",
            "assertions": [{"type": "jsonpath", "expression": "$.message", "operator": "equals", "expected": "登录成功"}],
        },
    )

    with patch.object(ApiRunner, "_validate_target"), patch("urllib.request.urlopen", return_value=response):
        result = ApiRunner().run(task)

    output = json.loads(result.output)
    assert result.exit_code == 0
    assert output["body"] == '{"message":"登录成功","user":"张三"}'
    assert output["responseSize"] == len(payload)
    assert output["compressedResponseSize"] == len(gzip.compress(payload))


def test_api_runner_uses_declared_chinese_charset():
    payload = '{"message":"登录成功"}'.encode("gb18030")
    response = FakeResponse(payload, {"Content-Type": "application/json; charset=gb18030"})
    task = TaskCreate(taskId="api-gb18030", type="api", payload={"url": "https://example.com/login"})

    with patch.object(ApiRunner, "_validate_target"), patch("urllib.request.urlopen", return_value=response):
        result = ApiRunner().run(task)

    assert json.loads(result.output)["body"] == '{"message":"登录成功"}'
