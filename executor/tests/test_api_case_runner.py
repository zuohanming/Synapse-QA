import json

from app.models.task import TaskCreate
from app.runners.api_case_runner import ApiCaseRunner


class FakeApiRunner:
    def __init__(self):
        self.calls = []

    def run(self, task, _progress, _canceled):
        self.calls.append(task.payload)
        index = len(self.calls)
        if index == 1:
            return type("Result", (), {
                "exit_code": 0,
                "output": json.dumps({"statusCode": 200, "durationMs": 12, "extractedVariables": {"token": "abc"}}),
                "error": "",
            })()
        return type("Result", (), {
            "exit_code": 0,
            "output": json.dumps({"statusCode": 201, "durationMs": 8, "extractedVariables": {}}),
            "error": "",
        })()


def test_api_case_runner_passes_extracted_variables_to_later_steps():
    runner = ApiCaseRunner()
    fake = FakeApiRunner()
    runner._api = fake
    task = TaskCreate(type="api_case", payload={
        "steps": [
            {"id": "1", "key": "login", "name": "登录", "enabled": True, "request": {"method": "POST", "url": "https://example.com/login"}},
            {"id": "2", "key": "create", "name": "创建", "enabled": True, "request": {"method": "POST", "url": "https://example.com/items", "headers": {"Authorization": "Bearer ${token}"}}},
        ],
        "variables": {},
    })

    result = runner.run(task)

    assert result.exit_code == 0
    assert fake.calls[1]["headers"]["Authorization"] == "Bearer abc"
    output = json.loads(result.output)
    assert output["status"] == "success"
    assert len(output["steps"]) == 2


def test_api_case_runner_runs_cleanup_after_failure():
    runner = ApiCaseRunner()

    class FailingApi:
        def run(self, task, _progress, _canceled):
            status = 500 if "main" in task.payload["url"] else 204
            return type("Result", (), {
                "exit_code": 0,
                "output": json.dumps({"statusCode": status, "durationMs": 1, "extractedVariables": {}}),
                "error": "",
            })()

    runner._api = FailingApi()
    task = TaskCreate(type="api_case", payload={
        "steps": [
            {"key": "main", "name": "主步骤", "enabled": True, "failurePolicy": "stop", "request": {"url": "https://example.com/main"}},
            {"key": "skip", "name": "后续", "enabled": True, "request": {"url": "https://example.com/skip"}},
            {"key": "cleanup", "name": "清理", "enabled": True, "phase": "cleanup", "failurePolicy": "always", "request": {"url": "https://example.com/cleanup"}},
        ]
    })

    output = json.loads(runner.run(task).output)

    assert output["steps"][0]["status"] == "failed"
    assert output["steps"][1]["status"] == "skipped"
    assert output["steps"][2]["status"] == "success"
