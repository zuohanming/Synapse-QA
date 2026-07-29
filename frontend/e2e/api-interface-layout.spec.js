import { expect, test } from "@playwright/test";

const apiBase = "http://127.0.0.1:8080/api";
const longPath = "/api/v1/auth/organizations/primary/workspaces/current/sessions/login-with-a-very-long-path";

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
            path: longPath,
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

async function expectFullyInsideCell(locator) {
  const bounds = await locator.evaluate((element) => {
    const elementRect = element.getBoundingClientRect();
    const cellRect = element.closest("td").getBoundingClientRect();
    const wrapRect = element.closest(".table-wrap").getBoundingClientRect();
    return {
      elementLeft: elementRect.left,
      elementRight: elementRect.right,
      elementTop: elementRect.top,
      elementBottom: elementRect.bottom,
      visibleLeft: Math.max(0, cellRect.left, wrapRect.left),
      visibleRight: Math.min(window.innerWidth, cellRect.right, wrapRect.right),
      visibleTop: Math.max(0, cellRect.top, wrapRect.top),
      visibleBottom: Math.min(window.innerHeight, cellRect.bottom, wrapRect.bottom)
    };
  });

  expect(bounds.elementLeft).toBeGreaterThanOrEqual(bounds.visibleLeft);
  expect(bounds.elementRight).toBeLessThanOrEqual(bounds.visibleRight);
  expect(bounds.elementTop).toBeGreaterThanOrEqual(bounds.visibleTop);
  expect(bounds.elementBottom).toBeLessThanOrEqual(bounds.visibleBottom);
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

  for (const name of ["调试", "编辑", "删除"]) {
    const button = page.getByRole("button", { name, exact: true });
    await expect(button).toBeVisible();
    const box = await button.boundingBox();
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(1440);
    await expectFullyInsideCell(button);
  }

  await expect(page.getByRole("checkbox", { name: "全选当前页接口" })).toBeVisible();
  const rowCheckbox = page.getByRole("checkbox", { name: "选择接口 登录接口" });
  await expect(rowCheckbox).toBeVisible();
  await expectFullyInsideCell(rowCheckbox);

  const pathContent = page.locator(".api-interface-list .data-table tbody td:nth-child(6) .table-cell-content");
  expect(await pathContent.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
  await pathContent.hover();
  await expect(page.getByRole("tooltip")).toContainText(longPath);
  await page.getByRole("columnheader", { name: "方法 / 路径" }).hover();
  await pathContent.focus();
  await expect(page.getByRole("tooltip")).toContainText(longPath);
});

test("接口管理按内容容器宽度降为单列且页面无横向溢出", async ({ page }) => {
  await mockApi(page);
  await page.addInitScript(() => {
    localStorage.setItem("synapse_qa_token", "e2e-token");
  });
  await page.setViewportSize({ width: 1100, height: 900 });

  await page.goto("/#/接口自动化/接口管理");
  const list = page.locator(".api-interface-list");
  const filters = page.locator(".api-interface-filter > *");
  await expect(filters).toHaveCount(7);
  expect(await list.evaluate((element) => element.clientWidth)).toBeLessThan(900);

  const filterTops = await filters.evaluateAll((elements) => elements.map((element) => element.getBoundingClientRect().top));
  expect(new Set(filterTops).size).toBe(7);

  const pageMetrics = await page.locator("html").evaluate((element) => ({
    scrollWidth: element.scrollWidth,
    clientWidth: element.clientWidth
  }));
  expect(pageMetrics.scrollWidth).toBeLessThanOrEqual(pageMetrics.clientWidth);
});
