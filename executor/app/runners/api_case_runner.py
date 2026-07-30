import json
import re
from typing import Callable

from app.models.task import TaskCreate, TaskResult
from app.runners.api_runner import ApiRunner
from app.runners.base import Runner


VARIABLE_PATTERN = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)\}")


class ApiCaseRunner(Runner):
    """在同一执行器内顺序运行接口用例实例，保证变量上下文不跨进程丢失。"""

    def __init__(self) -> None:
        self._api = ApiRunner()

    def run(self, task: TaskCreate, progress: Callable | None = None, canceled: Callable[[], bool] | None = None) -> TaskResult:
        steps = list(task.payload.get("steps") or [])
        variables = dict(task.payload.get("variables") or {})
        results: list[dict] = []
        stopped = False
        failed = False
        total = max(1, len(steps))

        for index, source in enumerate(steps):
            step = dict(source)
            phase = str(step.get("phase") or "main")
            policy = str(step.get("failurePolicy") or "stop")
            if not step.get("enabled", True):
                results.append(self._step_result(step, "skipped", "步骤已禁用"))
                continue
            if stopped and phase != "cleanup" and policy != "always":
                results.append(self._step_result(step, "skipped", "前序步骤失败"))
                continue
            if canceled and canceled() and phase != "cleanup" and policy != "always":
                stopped = True
                results.append(self._step_result(step, "canceled", "任务已取消"))
                continue
            condition = str(step.get("condition") or "").strip()
            if condition and not self._condition(condition, variables):
                results.append(self._step_result(step, "skipped", "执行条件不满足"))
                continue

            base_progress = round(index / total * 90)
            if progress:
                progress("case.step.started", "case", f"步骤 {index + 1}：{step.get('name') or step.get('key')} 开始执行", base_progress, {"stepIndex": index, "stepKey": step.get("key")})
            payload = self._resolve(step.get("request") or {}, variables)
            payload["extractors"] = step.get("extractors") or payload.get("extractors") or []
            payload["assertions"] = step.get("assertions") or payload.get("assertions") or []
            api_task = TaskCreate(taskId=f"{task.task_id}-{index + 1}", type="api", payload=payload)
            result = self._api.run(api_task, None, canceled)
            output = json.loads(result.output or "{}")
            status_code = int(output.get("statusCode") or 0)
            succeeded = result.exit_code == 0 and 200 <= status_code <= 299
            extracted = output.get("extractedVariables") or {}
            variables.update(extracted)
            variables[f"step_{step.get('key')}_status_code"] = status_code
            variables[f"step_{step.get('key')}_duration_ms"] = output.get("durationMs")
            step_result = {
                "stepId": step.get("id"),
                "stepKey": step.get("key"),
                "name": step.get("name"),
                "status": "success" if succeeded else "failed",
                "attempt": 1,
                "request": self._mask_request(payload),
                "response": output,
                "extractedVariables": extracted,
                "error": result.error if result.exit_code else "" if succeeded else f"HTTP 状态码 {status_code} 不在 2xx 范围",
            }
            results.append(step_result)
            if progress:
                progress("case.step.completed", "case", f"步骤 {index + 1}{'通过' if succeeded else '失败'}", round((index + 1) / total * 90), {"stepIndex": index, "stepKey": step.get("key"), "status": step_result["status"]})
            if not succeeded:
                failed = True
                if policy == "stop":
                    stopped = True

        result_payload = {
            "status": "failed" if failed else "canceled" if canceled and canceled() else "success",
            "steps": results,
            "variables": self._mask_variables(variables, set(task.payload.get("sensitiveVariables") or [])),
        }
        if progress:
            progress("case.completed", "case", "接口用例实例执行完成", 100, {"status": result_payload["status"]})
        return TaskResult(
            exit_code=0 if result_payload["status"] == "success" else 1,
            output=json.dumps(result_payload, ensure_ascii=False),
            error="" if not failed else "接口用例存在失败步骤",
        )

    @staticmethod
    def _resolve(value, variables):
        if isinstance(value, dict):
            return {key: ApiCaseRunner._resolve(item, variables) for key, item in value.items()}
        if isinstance(value, list):
            return [ApiCaseRunner._resolve(item, variables) for item in value]
        if not isinstance(value, str):
            return value

        def replace(match):
            name = match.group(1)
            if name not in variables:
                raise ValueError(f"变量 {name} 未定义")
            raw = variables[name]
            return raw if isinstance(raw, str) else json.dumps(raw, ensure_ascii=False, separators=(",", ":"))

        return VARIABLE_PATTERN.sub(replace, value)

    @staticmethod
    def _condition(expression: str, variables: dict) -> bool:
        resolved = ApiCaseRunner._resolve(expression, variables).strip()
        match = re.fullmatch(r"""(.+?)\s*(==|!=)\s*["']?(.*?)["']?""", resolved)
        if not match:
            return resolved.lower() not in {"", "false", "0", "none", "null"}
        left, operator, right = match.groups()
        return (left.strip() == right.strip()) if operator == "==" else (left.strip() != right.strip())

    @staticmethod
    def _step_result(step: dict, status: str, message: str) -> dict:
        return {"stepId": step.get("id"), "stepKey": step.get("key"), "name": step.get("name"), "status": status, "message": message}

    @staticmethod
    def _mask_request(payload: dict) -> dict:
        masked = dict(payload)
        headers = {}
        for key, value in (payload.get("headers") or {}).items():
            headers[key] = "******" if key.lower() in {"authorization", "cookie", "set-cookie", "x-api-key", "satoken"} else value
        masked["headers"] = headers
        return masked

    @staticmethod
    def _mask_variables(variables: dict, sensitive: set[str]) -> dict:
        return {key: "******" if key in sensitive else value for key, value in variables.items()}
