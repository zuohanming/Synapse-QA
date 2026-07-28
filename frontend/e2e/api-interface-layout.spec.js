import { expect, test } from "@playwright/test";

const apiBase = "http://127.0.0.1:8080/api";

async function mockApi(page) {
  await page.route(`${apiBase}/auth/me`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { id: 1, username: "admin", nickname: "管理员" } })
    });
  });

  await page.route(`${apiBase}/config/projects**`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { items: [{ id: 1, name: "示例项目" }], total: 1 } })
    });
  });

  await page.route(`${apiBase}/config/products**`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { items: [{ id: 10, projectId: 1, projectName: "示例项目", name: "示例产品" }], total: 1 } })
    });
  });

  await page.route(`${apiBase}/api-automation/interfaces**`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          items: [{
            id: 1,
            projectId: 1,
            projectName: "示例项目",
            productId: 10,
            productName: "示例产品",
            moduleName: "登录模块",
            name: "登录接口",
            path: "/api/v1/auth/login",
            method: "POST",
            endpointType: "WEB",
            lifecycleStatus: "active",
            updatedBy: "admin",
            updatedAt: "2026-07-28T10:00:00Z"
          }],
          total: 1,
          page: 1,
          pageSize: 20
        }
      })
    });
  });
}

test("接口管理在桌面端保持单行搜索且列表无横向溢出", async ({ page }) => {
  await mockApi(page);
  await page.addInitScript(() => {
    localStorage.setItem("synapse_qa_token", "e2e-token");
  });
  await page.setViewportSize({ width: 1440, height: 900 });

  await page.goto("/#/接口自动化/接口管理");
  const filters = page.locator(".api-interface-filter > *");
  await expect(filters).toHaveCount(7);
  await expect(page.locator(".api-interface-list .data-table tbody tr")).toHaveCount(1);

  const filterTops = await filters.evaluateAll((elements) => elements.map((element) => element.getBoundingClientRect().top));
  expect(filterTops).toEqual(Array(filterTops.length).fill(filterTops[0]));

  const tableMetrics = await page.locator(".api-interface-list .table-wrap").evaluate((element) => ({
    scrollWidth: element.scrollWidth,
    clientWidth: element.clientWidth
  }));
  expect(tableMetrics.scrollWidth).toBeLessThanOrEqual(tableMetrics.clientWidth);

  const updatedHeader = page.getByRole("columnheader", { name: "最近修改" });
  await expect(updatedHeader).toBeVisible();
  await expect(updatedHeader).toBeInViewport({ ratio: 1 });
  expect(await updatedHeader.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);

  const operationsHeader = page.getByRole("columnheader", { name: "操作" });
  await expect(operationsHeader).toBeVisible();
  await expect(operationsHeader).toBeInViewport({ ratio: 1 });
  expect(await operationsHeader.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
});
