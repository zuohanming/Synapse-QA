from pathlib import Path

from playwright.sync_api import sync_playwright


errors = []
artifact = Path(__file__).parent / "artifacts" / "execution-mode-modal.png"

with sync_playwright() as playwright:
    browser = playwright.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 900})
    page.on("console", lambda message: errors.append(message.text) if message.type == "error" else None)
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.goto("http://127.0.0.1:4173")
    page.wait_for_load_state("networkidle")
    if page.get_by_role("button", name="登录").count():
        page.get_by_role("button", name="登录").click()
        page.wait_for_load_state("networkidle")
    page.get_by_text("测试用例", exact=True).click()
    page.wait_for_load_state("networkidle")
    page.get_by_role("button", name="执行用例 测试登录成功，并跳转至统一认证平台").click()
    modal = page.get_by_role("region", name="选择执行模式")
    assert modal.is_visible()
    assert modal.get_by_role("button", name="无头执行").get_attribute("aria-pressed") == "true"
    artifact.parent.mkdir(parents=True, exist_ok=True)
    page.screenshot(path=str(artifact), full_page=True)
    browser.close()

assert not errors, errors
print(f"execution mode modal: ok; screenshot={artifact}")
