from pathlib import Path

from playwright.sync_api import sync_playwright


errors = []
artifact = Path(__file__).parent / "artifacts" / "canvas-node-types.png"

with sync_playwright() as playwright:
    browser = playwright.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1600, "height": 1000})
    page.on("console", lambda message: errors.append(message.text) if message.type == "error" else None)
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.goto("http://127.0.0.1:4173")
    page.wait_for_load_state("networkidle")
    if page.get_by_role("button", name="登录").count():
        page.get_by_role("button", name="登录").click()
        page.wait_for_load_state("networkidle")

    page.get_by_text("页面步骤", exact=True).click()
    page.wait_for_load_state("networkidle")
    page.locator("tbody tr").first.get_by_role("button", name="调试").click()
    page.wait_for_load_state("networkidle")

    canvas = page.locator(".flow-canvas")
    node_types = ["断言操作", "SQL操作", "自定义变量", "条件判断", "python代码"]
    expected_operations = ["页面文本包含", "执行 PostgreSQL 查询", "设置变量", "变量条件判断", "执行 Python 代码"]
    for index, (node_type, operation) in enumerate(zip(node_types, expected_operations)):
        page.get_by_role("button", name=node_type).drag_to(canvas, target_position={"x": 130 + index * 105, "y": 500})
        page.get_by_role("button", name="请选择节点操作").click()
        if node_type == "断言操作":
            assert page.locator(".operation-item-list button").count() == 19
        assert page.get_by_role("button", name=operation).is_visible()
        page.get_by_role("button", name=operation).click()

    assert page.get_by_label("从变量条件判断真分支开始连接").is_visible()
    assert page.get_by_label("从变量条件判断假分支开始连接").is_visible()
    artifact.parent.mkdir(parents=True, exist_ok=True)
    page.screenshot(path=str(artifact), full_page=True)
    browser.close()

assert not errors, errors
print(f"canvas node types: ok; screenshot={artifact}")
