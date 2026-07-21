"""创建完整业务数据、执行 UI 用例并生成报告的端到端验收脚本。"""

import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime
from pathlib import Path


API_BASE = os.getenv("SYNAPSE_API_BASE", "http://127.0.0.1:8080/api")
USERNAME = os.getenv("SYNAPSE_USERNAME", "admin")
PASSWORD = os.getenv("SYNAPSE_PASSWORD", "admin123")
TARGET_URL = os.getenv("SYNAPSE_E2E_TARGET", "http://127.0.0.1:4173/e2e-target.html")


def request(path, method="GET", body=None, token=None):
    data = json.dumps(body, ensure_ascii=False).encode("utf-8") if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(API_BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            payload = json.loads(response.read().decode("utf-8"))
            return payload.get("data")
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8")
        raise RuntimeError(f"{method} {path} 失败：{error.code} {detail}") from error


def query_id(path, name, token, name_key="name", params=None):
    query = {"page": 1, "pageSize": 100, **(params or {})}
    result = request(f"{path}?{urllib.parse.urlencode(query)}", token=token)
    for item in result.get("items", []):
        if item.get(name_key) == name:
            return item["id"]
    raise RuntimeError(f"未找到刚创建的数据：{name}")


def main():
    token = request("/auth/login", "POST", {"username": USERNAME, "password": PASSWORD})["token"]
    suffix = datetime.now().strftime("%Y%m%d-%H%M%S")
    project_name = f"E2E-项目-{suffix}"
    product_name = f"E2E-产品-{suffix}"
    module_name = f"E2E-模块-{suffix}"
    page_name = f"E2E-目标页-{suffix}"
    element_name = f"E2E-状态元素-{suffix}"
    step_name = f"E2E-标题断言-{suffix}"
    case_name = f"E2E-完整流程-{suffix}"

    request("/config/projects", "POST", {"name": project_name, "status": "active"}, token)
    project_id = query_id("/config/projects", project_name, token)

    request("/config/products", "POST", {"projectId": project_id, "name": product_name, "uiType": "WEB", "apiType": "NONE"}, token)
    product_id = query_id("/config/products", product_name, token)

    request("/config/product-modules", "POST", {"productId": product_id, "name": module_name, "level1": "质量验证", "level2": "闭环"}, token)
    module_id = query_id("/config/product-modules", module_name, token, params={"productId": product_id})

    request("/config/test-objects", "POST", {"productId": product_id, "envName": "本地验收", "target": TARGET_URL, "deployEnv": "测试环境", "autoType": "界面自动化", "owner": USERNAME, "queryEnabled": True, "writeEnabled": False}, token)

    product_path = f"{project_name}/{product_name}"
    request("/ui/elements", "POST", {"name": page_name, "category": product_path, "method": module_name, "locator": TARGET_URL, "action": "", "value": "WEB", "description": "E2E 测试目标页面", "status": "active"}, token)
    page_id = query_id("/ui/elements", page_name, token)

    request("/ui/page-elements", "POST", {"pageId": page_id, "name": element_name, "type1": "css", "locator1": "#status", "index1": "", "type2": "", "locator2": "", "index2": "", "type3": "", "locator3": "", "index3": "", "aiPrompt": "", "waitTime": "5"}, token)

    request("/ui/steps", "POST", {"name": step_name, "category": product_path, "method": module_name, "locator": page_name, "action": "assertTitle", "value": "Synapse QA E2E Target", "description": "断言测试目标页面标题", "status": "active"}, token)
    step_id = query_id("/ui/steps", step_name, token)

    case = request("/test-cases", "POST", {"productId": product_id, "moduleId": module_id, "pageId": page_id, "name": case_name, "caseType": "ui", "priority": "P0", "status": "active", "owner": USERNAME, "tags": "e2e,smoke", "description": "项目到报告完整闭环验收", "preconditions": TARGET_URL, "expectedResult": "页面标题断言通过", "dataEnabled": False, "stepIds": [step_id]}, token)
    case_id = case["id"]

    run = request("/executions", "POST", {"runType": "ui", "caseIds": [case_id]}, token)
    run_id = run["id"]
    deadline = time.time() + 30
    while time.time() < deadline:
        report = request(f"/executions/{run_id}", token=token)
        if report["status"] in {"completed", "failed", "canceled"}:
            break
        time.sleep(0.5)
    else:
        raise RuntimeError(f"执行批次 #{run_id} 在 30 秒内未结束")

    if report["status"] != "completed" or report.get("summary", {}).get("passed") != 1:
        raise RuntimeError(f"执行未通过：{json.dumps(report, ensure_ascii=False)}")

    artifacts_dir = Path(__file__).parent / "artifacts"
    artifacts_dir.mkdir(exist_ok=True)
    report_path = artifacts_dir / f"e2e-report-{run_id}.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"projectId": project_id, "productId": product_id, "moduleId": module_id, "pageId": page_id, "stepId": step_id, "caseId": case_id, "runId": run_id, "status": report["status"], "report": str(report_path)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
