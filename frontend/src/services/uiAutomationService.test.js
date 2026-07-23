import { afterEach, describe, expect, it, vi } from "vitest";

import { uiAutomationService } from "./uiAutomationService.js";

function ok(data = {}) {
  return Promise.resolve({ ok: true, json: () => Promise.resolve({ data }) });
}

describe("uiAutomationService.cases", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("封装测试用例 CRUD、导入导出和参数化接口", async () => {
    global.fetch = vi.fn(() => ok({}));
    await uiAutomationService.cases.list({ name: "登录" });
    await uiAutomationService.cases.get(1);
    await uiAutomationService.cases.create({ name: "新增" });
    await uiAutomationService.cases.update(1, { name: "编辑" });
    await uiAutomationService.cases.remove(1);
    await uiAutomationService.cases.import([{ name: "导入" }]);
    await uiAutomationService.cases.export({ status: "active" });
    await uiAutomationService.cases.datasets.list(1);
    await uiAutomationService.cases.datasets.create(1, { name: "默认", variables: {} });
    await uiAutomationService.cases.datasets.update(1, 2, { name: "默认", variables: {} });
    await uiAutomationService.cases.datasets.remove(1, 2);

    const urls = global.fetch.mock.calls.map((item) => String(item[0]));
    expect(urls).toContain("http://127.0.0.1:8080/api/test-cases?name=%E7%99%BB%E5%BD%95");
    expect(urls).toContain("http://127.0.0.1:8080/api/test-cases/1");
    expect(urls).toContain("http://127.0.0.1:8080/api/test-cases/import");
    expect(urls).toContain("http://127.0.0.1:8080/api/test-cases/export?status=active");
    expect(urls).toContain("http://127.0.0.1:8080/api/test-cases/1/datasets/2");
  });
});
