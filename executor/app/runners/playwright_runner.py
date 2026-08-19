import json
import re
import subprocess
import sys
from pathlib import Path
from uuid import uuid4

from app.core.config import settings
from app.core.subprocess_utils import hidden_subprocess_kwargs
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class PlaywrightRunner(Runner):
    """运行轻量级 Playwright UI 任务，支持常用页面操作和断言。"""

    def run(self, task: TaskCreate, on_progress=None) -> TaskResult:
        try:
            from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
            from playwright.sync_api import sync_playwright
        except ImportError as error:
            raise RuntimeError("缺少 playwright 依赖，请先安装 executor/requirements.txt") from error

        payload = task.payload
        actions = payload.get("actions") or []
        if not isinstance(actions, list):
            raise ValueError("ui 任务的 payload.actions 必须是数组")

        timeout_ms = int(payload.get("timeoutSeconds") or settings.default_timeout_seconds) * 1000
        browser_name = str(payload.get("browser") or "chromium")
        headless = bool(payload.get("headless", True))
        artifacts_dir = Path(payload.get("artifactsDir") or settings.artifacts_dir).resolve()
        artifacts_dir.mkdir(parents=True, exist_ok=True)
        artifacts: list[str] = []
        logs: list[str] = []
        datasets = payload.get("datasets") or [{"name": "", "variables": {}}]
        if not isinstance(datasets, list) or any(not isinstance(item, dict) for item in datasets):
            raise ValueError("ui 任务的 payload.datasets 必须是对象数组")

        try:
            with sync_playwright() as playwright:
                browser_type = getattr(playwright, browser_name, None)
                if browser_type is None:
                    raise ValueError("ui 任务的 payload.browser 只能是 chromium、firefox 或 webkit")

                browser = browser_type.launch(headless=headless)
                try:
                    for dataset_index, dataset in enumerate(datasets, start=1):
                        variables = dict(dataset.get("variables") or {})
                        dataset_name = str(dataset.get("name") or f"数据集 {dataset_index}")
                        if payload.get("datasets"):
                            self._report_step(logs, f"参数化数据：{dataset_name}", "开始", on_progress)
                        context = browser.new_context(viewport=payload.get("viewport"))
                        try:
                            page = context.new_page()
                            page.set_default_timeout(timeout_ms)
                            url = payload.get("url")
                            if url:
                                self._report_step(logs, str(payload.get("urlLabel") or f"打开 {url}"), "开始", on_progress)
                                page.goto(str(url), wait_until=str(payload.get("waitUntil") or "load"))
                                self._report_step(logs, str(payload.get("urlLabel") or f"打开 {url}"), "完成", on_progress)
                            self._run_actions(page, actions, artifacts_dir, artifacts, logs, variables, on_progress)
                            if payload.get("datasets"):
                                self._report_step(logs, f"参数化数据：{dataset_name}", "完成", on_progress)
                        finally:
                            context.close()
                finally:
                    browser.close()
        except PlaywrightTimeoutError as error:
            return TaskResult(exit_code=1, output="\n".join(logs), error=f"Playwright 超时: {error}", artifacts=artifacts)

        return TaskResult(exit_code=0, output="\n".join(logs), artifacts=artifacts)

    def _run_actions(self, page, actions: list[dict], artifacts_dir: Path, artifacts: list[str], logs: list[str], variables: dict, on_progress=None) -> None:
        graph_actions = {str(action.get("nodeId")): action for action in actions if action.get("nodeId") is not None}
        if not graph_actions:
            for action in actions:
                self._run_action(page, action, artifacts_dir, artifacts, logs, variables, on_progress)
            return

        incoming = {
            str(target)
            for action in actions
            for target in (action.get("next"), action.get("trueNext"), action.get("falseNext"))
            if target not in (None, "")
        }
        current = next((str(action.get("nodeId")) for action in actions if str(action.get("nodeId")) not in incoming), str(actions[0].get("nodeId")))
        visited: set[str] = set()
        while current and current in graph_actions:
            if current in visited:
                raise ValueError("画布调试流程不能包含循环")
            visited.add(current)
            action = graph_actions[current]
            condition_result = self._run_action(page, action, artifacts_dir, artifacts, logs, variables, on_progress)
            if action.get("action") == "condition":
                current = str(action.get("trueNext") if condition_result else action.get("falseNext") or "")
            else:
                current = str(action.get("next") or "")

    def _run_action(self, page, action: dict, artifacts_dir: Path, artifacts: list[str], logs: list[str], variables: dict | None = None, on_progress=None) -> bool | None:
        if not isinstance(action, dict):
            raise ValueError("ui 任务的 actions 每一项都必须是对象")

        variables = variables if variables is not None else {}
        name = str(action.get("action") or "")
        selector = self._resolve(action.get("selector"), variables)
        value = self._resolve(action.get("value"), variables)
        label = str(action.get("label") or "")
        step_label = label or self._default_action_label(name, action)
        self._report_step(logs, step_label, "开始", on_progress)

        if name == "goto":
            url = action.get("url")
            if not url:
                raise ValueError("goto 动作必须提供 url")
            page.goto(str(url), wait_until=str(action.get("waitUntil") or "load"))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "click":
            self._require_selector(selector, name)
            page.click(str(selector))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "dblclick":
            self._require_selector(selector, name)
            page.dblclick(str(selector))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "hover":
            self._require_selector(selector, name)
            page.hover(str(selector))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "fill":
            self._require_selector(selector, name)
            page.fill(str(selector), "" if value is None else str(value))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "press":
            self._require_selector(selector, name)
            if value is None:
                raise ValueError("press 动作必须提供 value")
            page.press(str(selector), str(value))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "waitForSelector":
            self._require_selector(selector, name)
            page.wait_for_selector(str(selector))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "waitForTimeout":
            seconds = float(value or 0)
            if seconds < 0:
                raise ValueError("waitForTimeout 动作的等待时间不能小于 0")
            page.wait_for_timeout(seconds * 1000)
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertText":
            expected = self._resolve(action.get("text"), variables)
            if expected is None:
                raise ValueError("assertText 动作必须提供 text")
            content = page.locator(str(selector)).inner_text() if selector else page.locator("body").inner_text()
            if str(expected) not in content:
                raise AssertionError(f"页面文本断言失败，未找到: {expected}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertTitle":
            expected = self._resolve(action.get("text"), variables)
            if expected is None:
                raise ValueError("assertTitle 动作必须提供 text")
            title = page.title()
            if str(expected) not in title:
                raise AssertionError(f"页面标题断言失败，实际标题: {title}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertTitleEquals":
            expected = self._resolve(action.get("text"), variables)
            if page.title() != str(expected):
                raise AssertionError(f"页面标题等于断言失败，实际标题: {page.title()}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertURL":
            expected = self._resolve(action.get("text"), variables)
            if str(expected) not in page.url:
                raise AssertionError(f"页面 URL 断言失败，实际 URL: {page.url}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "screenshot":
            file_name = str(action.get("name") or f"ui-{uuid4().hex}.png")
            path = artifacts_dir / file_name
            page.screenshot(path=path, full_page=bool(action.get("fullPage", True)))
            artifacts.append(str(path))
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertElementExists":
            self._require_selector(selector, name)
            if page.locator(str(selector)).count() < 1:
                raise AssertionError(f"元素存在断言失败: {selector}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertTextEquals":
            self._require_selector(selector, name)
            expected = self._resolve(action.get("text"), variables)
            actual = page.locator(str(selector)).inner_text()
            if actual != str(expected):
                raise AssertionError(f"元素文本等于断言失败，实际文本: {actual}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name in {"assertVisible", "assertHidden", "assertEnabled", "assertDisabled", "assertChecked", "assertUnchecked"}:
            self._require_selector(selector, name)
            locator = page.locator(str(selector))
            checks = {
                "assertVisible": (locator.is_visible, "可见"),
                "assertHidden": (locator.is_hidden, "隐藏"),
                "assertEnabled": (locator.is_enabled, "启用"),
                "assertDisabled": (locator.is_disabled, "禁用"),
                "assertChecked": (locator.is_checked, "已选中"),
                "assertUnchecked": (lambda: not locator.is_checked(), "未选中"),
            }
            check, expected_state = checks[name]
            if not check():
                raise AssertionError(f"元素状态断言失败，期望状态: {expected_state}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertValueEquals":
            self._require_selector(selector, name)
            expected = self._resolve(action.get("text"), variables)
            actual = page.locator(str(selector)).input_value()
            if actual != str(expected):
                raise AssertionError(f"输入值断言失败，实际值: {actual}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertAttribute":
            self._require_selector(selector, name)
            attribute = str(action.get("attribute") or "").strip()
            if not attribute:
                raise ValueError("元素属性断言必须提供属性名称")
            actual = page.locator(str(selector)).get_attribute(attribute)
            expected = self._resolve(action.get("text"), variables)
            if not self._compare(actual, str(action.get("operator") or "equals"), expected):
                raise AssertionError(f"元素属性断言失败，实际值: {actual}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertCount":
            self._require_selector(selector, name)
            try:
                expected_count = int(self._resolve(action.get("count"), variables))
            except (TypeError, ValueError) as error:
                raise ValueError("元素数量断言的期望数量必须是整数") from error
            actual_count = page.locator(str(selector)).count()
            if actual_count != expected_count:
                raise AssertionError(f"元素数量断言失败，实际数量: {actual_count}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertVariable":
            if not self._compare(self._resolve(action.get("left"), variables), str(action.get("operator") or "equals"), self._resolve(action.get("right"), variables)):
                raise AssertionError("变量断言失败")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertVariableExists":
            variable_name = str(action.get("name") or "").strip()
            if not variable_name or variable_name not in variables:
                raise AssertionError(f"变量存在断言失败: {variable_name}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "assertRegex":
            actual = str(self._resolve(action.get("value"), variables))
            pattern = str(self._resolve(action.get("pattern"), variables) or "")
            try:
                matched = re.search(pattern, actual) is not None
            except re.error as error:
                raise ValueError(f"正则表达式无效: {error}") from error
            if not matched:
                raise AssertionError(f"正则匹配断言失败，实际值: {actual}")
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "setVariable":
            variable_name = str(action.get("name") or "").strip()
            if not variable_name:
                raise ValueError("设置变量必须提供变量名")
            variables[variable_name] = self._resolve(action.get("value"), variables)
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "condition":
            matched = self._compare(self._resolve(action.get("left"), variables), str(action.get("operator") or "equals"), self._resolve(action.get("right"), variables))
            self._report_step(logs, f"{step_label}：{'真' if matched else '假'}", "完成", on_progress)
            return matched
        elif name == "sqlQuery":
            self._run_sql(action, variables)
            self._report_step(logs, step_label, "完成", on_progress)
        elif name == "pythonCode":
            self._run_python(action, variables)
            self._report_step(logs, step_label, "完成", on_progress)
        else:
            raise ValueError(f"不支持的 ui 动作: {name}")

        return None

    def _append_log(self, logs: list[str], message: str, on_progress=None) -> None:
        logs.append(message)
        if on_progress:
            on_progress("\n".join(logs))

    def _report_step(self, logs: list[str], label: str, status: str, on_progress=None) -> None:
        self._append_log(logs, f"[{status}] {label}", on_progress)

    def _default_action_label(self, name: str, action: dict) -> str:
        labels = {
            "click": "点击元素",
            "dblclick": "双击元素",
            "hover": "悬停元素",
            "fill": "输入内容",
            "waitForSelector": "等待元素",
            "screenshot": "页面截图",
        }
        if name == "goto":
            return f"打开 {action.get('url') or 'URL'}"
        if name == "press":
            return f"按键 {action.get('value')}"
        if name == "waitForTimeout":
            return f"等待 {float(action.get('value') or 0):g} 秒"
        if name == "assertText":
            return f"校验文本 {action.get('text')}"
        if name == "assertTitle":
            return f"校验标题 {action.get('text')}"
        return labels.get(name, name or "未知步骤")

    def _resolve(self, value, variables: dict):
        if not isinstance(value, str):
            return value
        full_match = re.fullmatch(r"\$\{([^{}]+)\}", value)
        if full_match:
            return variables.get(full_match.group(1), "")

        def replace(match) -> str:
            resolved = variables.get(match.group(1), "")
            return json.dumps(resolved, ensure_ascii=False, default=str) if isinstance(resolved, (dict, list)) else str(resolved)

        return re.sub(r"\$\{([^{}]+)\}", replace, value)

    def _compare(self, left, operator: str, right) -> bool:
        if operator == "contains":
            return str(right) in str(left)
        if operator in {"greaterThan", "lessThan"}:
            try:
                left, right = float(left), float(right)
            except (TypeError, ValueError):
                left, right = str(left), str(right)
            return left > right if operator == "greaterThan" else left < right
        if operator == "notEquals":
            return str(left) != str(right)
        return str(left) == str(right)

    def _run_sql(self, action: dict, variables: dict) -> None:
        try:
            import psycopg2
            from psycopg2.extras import RealDictCursor
        except ImportError as error:
            raise RuntimeError("执行 SQL 节点需要安装 psycopg2-binary") from error

        connection_string = str(self._resolve(action.get("connectionString"), variables) or "").strip()
        query = str(self._resolve(action.get("query"), variables) or "").strip()
        result_variable = str(action.get("resultVariable") or "").strip()
        if not connection_string or not query or not result_variable:
            raise ValueError("SQL 节点必须配置连接串、查询语句和结果变量名")
        with psycopg2.connect(connection_string, connect_timeout=10) as connection:
            connection.set_session(readonly=True)
            with connection.cursor(cursor_factory=RealDictCursor) as cursor:
                cursor.execute("set statement_timeout = 10000")
                cursor.execute(query)
                rows = cursor.fetchall() if cursor.description else []
        variables[result_variable] = json.loads(json.dumps(rows, ensure_ascii=False, default=str))

    def _run_python(self, action: dict, variables: dict) -> None:
        code = str(action.get("code") or "")
        if not code.strip():
            raise ValueError("Python 节点代码不能为空")
        wrapper = (
            "import contextlib,io,json,sys\n"
            "payload=json.load(sys.stdin)\nscope={'variables':payload['variables'],'result':{}}\nbuffer=io.StringIO()\n"
            "with contextlib.redirect_stdout(buffer): exec(compile(payload['code'],'<canvas-python>','exec'),scope,scope)\n"
            "json.dump({'variables':scope['variables'],'result':scope['result'],'output':buffer.getvalue()},sys.stdout,ensure_ascii=False,default=str)\n"
        )
        try:
            completed = subprocess.run(
                [sys.executable, "-I", "-c", wrapper],
                input=json.dumps({"variables": variables, "code": code}, ensure_ascii=False, default=str),
                capture_output=True,
                text=True,
                timeout=10,
                check=False,
                **hidden_subprocess_kwargs(),
            )
        except subprocess.TimeoutExpired as error:
            raise RuntimeError("Python 节点执行超过 10 秒") from error
        if completed.returncode != 0:
            raise RuntimeError(completed.stderr.strip() or "Python 节点执行失败")
        payload = json.loads(completed.stdout)
        variables.update(payload.get("variables") or {})
        variables.update(payload.get("result") or {})

    def _require_selector(self, selector: object, action_name: str) -> None:
        if not selector:
            raise ValueError(f"{action_name} 动作必须提供 selector")
