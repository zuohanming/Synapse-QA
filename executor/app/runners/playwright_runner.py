from pathlib import Path
from uuid import uuid4

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class PlaywrightRunner(Runner):
    """运行轻量级 Playwright UI 任务，支持常用页面操作和断言。"""

    def run(self, task: TaskCreate) -> TaskResult:
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

        try:
            with sync_playwright() as playwright:
                browser_type = getattr(playwright, browser_name, None)
                if browser_type is None:
                    raise ValueError("ui 任务的 payload.browser 只能是 chromium、firefox 或 webkit")

                browser = browser_type.launch(headless=headless)
                try:
                    context = browser.new_context(viewport=payload.get("viewport"))
                    page = context.new_page()
                    page.set_default_timeout(timeout_ms)

                    url = payload.get("url")
                    if url:
                        page.goto(str(url), wait_until=str(payload.get("waitUntil") or "load"))
                        logs.append(f"goto {url}")

                    for action in actions:
                        self._run_action(page, action, artifacts_dir, artifacts, logs)
                finally:
                    browser.close()
        except PlaywrightTimeoutError as error:
            return TaskResult(exit_code=1, output="\n".join(logs), error=f"Playwright 超时: {error}", artifacts=artifacts)

        return TaskResult(exit_code=0, output="\n".join(logs), artifacts=artifacts)

    def _run_action(self, page, action: dict, artifacts_dir: Path, artifacts: list[str], logs: list[str]) -> None:
        if not isinstance(action, dict):
            raise ValueError("ui 任务的 actions 每一项都必须是对象")

        name = str(action.get("action") or "")
        selector = action.get("selector")
        value = action.get("value")

        if name == "goto":
            url = action.get("url")
            if not url:
                raise ValueError("goto 动作必须提供 url")
            page.goto(str(url), wait_until=str(action.get("waitUntil") or "load"))
            logs.append(f"goto {url}")
        elif name == "click":
            self._require_selector(selector, name)
            page.click(str(selector))
            logs.append(f"click {selector}")
        elif name == "fill":
            self._require_selector(selector, name)
            page.fill(str(selector), "" if value is None else str(value))
            logs.append(f"fill {selector}")
        elif name == "press":
            self._require_selector(selector, name)
            if value is None:
                raise ValueError("press 动作必须提供 value")
            page.press(str(selector), str(value))
            logs.append(f"press {selector} {value}")
        elif name == "waitForSelector":
            self._require_selector(selector, name)
            page.wait_for_selector(str(selector))
            logs.append(f"waitForSelector {selector}")
        elif name == "assertText":
            expected = action.get("text")
            if expected is None:
                raise ValueError("assertText 动作必须提供 text")
            content = page.locator(str(selector)).inner_text() if selector else page.locator("body").inner_text()
            if str(expected) not in content:
                raise AssertionError(f"页面文本断言失败，未找到: {expected}")
            logs.append(f"assertText {expected}")
        elif name == "assertTitle":
            expected = action.get("text")
            if expected is None:
                raise ValueError("assertTitle 动作必须提供 text")
            title = page.title()
            if str(expected) not in title:
                raise AssertionError(f"页面标题断言失败，实际标题: {title}")
            logs.append(f"assertTitle {expected}")
        elif name == "screenshot":
            file_name = str(action.get("name") or f"ui-{uuid4().hex}.png")
            path = artifacts_dir / file_name
            page.screenshot(path=path, full_page=bool(action.get("fullPage", True)))
            artifacts.append(str(path))
            logs.append(f"screenshot {path}")
        else:
            raise ValueError(f"不支持的 ui 动作: {name}")

    def _require_selector(self, selector: object, action_name: str) -> None:
        if not selector:
            raise ValueError(f"{action_name} 动作必须提供 selector")
