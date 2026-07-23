from pathlib import Path

from playwright.sync_api import sync_playwright


ARTIFACTS = Path(__file__).resolve().parent / "artifacts"
ARTIFACTS.mkdir(exist_ok=True)

with sync_playwright() as playwright:
    browser = playwright.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 900})
    console_errors = []
    page.on("console", lambda message: console_errors.append(message.text) if message.type == "error" else None)
    page.goto("http://127.0.0.1:4173")
    page.wait_for_load_state("networkidle")
    if page.get_by_label("用户名").count():
        page.get_by_label("用户名").fill("admin")
        page.get_by_label("密码").fill("admin123")
        page.get_by_role("button", name="登录").click()
        page.wait_for_load_state("networkidle")

    home = page.get_by_role("button", name="首页", exact=True)
    assert home.get_attribute("aria-expanded") == "false"
    page.screenshot(path=ARTIFACTS / "menu-groups-collapsed.png", full_page=True)

    home.click()
    page.wait_for_timeout(250)
    assert home.get_attribute("aria-expanded") == "true"
    assert page.get_by_role("button", name="项目概览", exact=True).is_visible()
    page.screenshot(path=ARTIFACTS / "menu-group-expanded.png", full_page=True)

    home.click()
    assert home.get_attribute("aria-expanded") == "false"
    assert not console_errors, console_errors
    browser.close()

print("sidebar menu groups: ok")
