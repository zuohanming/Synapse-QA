import { expect, test } from "@playwright/test";

const apiBase = "http://127.0.0.1:8080/api";

async function mockApi(page) {
  await page.route(`${apiBase}/auth/me`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { id: 1, username: "admin", nickname: "系统管理员" } })
    });
  });

  await page.route(`${apiBase}/config/products**`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { items: [{ id: 10, projectName: "演示DEMO", name: "模拟UI" }], total: 1 } })
    });
  });

  await page.route(`${apiBase}/config/product-modules**`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { items: [{ id: 20, name: "登录" }], total: 1 } })
    });
  });

  await page.route(`${apiBase}/ui/elements**`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { items: [{ id: 30, name: "登录页" }], total: 1 } })
    });
  });

  await page.route(`${apiBase}/ui/steps**`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { items: [{ id: 40, name: "输入账号" }], total: 1 } })
    });
  });

  await page.route(`${apiBase}/test-cases/1`, async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: 1,
            productName: "演示DEMO/模拟UI",
            moduleName: "登录",
            pageName: "登录页",
            name: "登录成功",
            priority: "P1",
            status: "active",
            owner: "admin",
            steps: [{ id: 1, stepId: 40, stepName: "输入账号" }],
            datasets: []
          }
        })
      });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: { message: "ok" } }) });
  });

  await page.route(`${apiBase}/test-cases**`, async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/api/test-cases/1") && route.request().method() === "GET") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: 1,
            productName: "演示DEMO/模拟UI",
            moduleName: "登录",
            pageName: "登录页",
            name: "登录成功",
            priority: "P1",
            status: "active",
            owner: "admin",
            steps: [{ id: 1, stepId: 40, stepName: "输入账号" }],
            datasets: []
          }
        })
      });
      return;
    }
    if (route.request().method() === "POST") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: { id: 2, message: "created" } }) });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          items: [
            {
              id: 1,
              productId: 10,
              moduleId: 20,
              pageId: 30,
              productName: "演示DEMO/模拟UI",
              moduleName: "登录",
              pageName: "登录页",
              name: "登录成功",
              priority: "P1",
              status: "active",
              owner: "admin",
              updatedAt: "2026-06-17T10:00:00Z"
            }
          ],
          total: 1,
          page: 1,
          pageSize: 20
        }
      })
    });
  });
}

test("测试用例页面关键路径", async ({ page }) => {
  await mockApi(page);
  await page.addInitScript(() => {
    localStorage.setItem("synapse_qa_token", "e2e-token");
  });

  await page.goto("/#/界面自动化/测试用例");
  await expect(page.getByRole("heading", { name: "测试用例" })).toBeVisible();
  await expect(page.getByText("登录成功")).toBeVisible();

  await page.getByRole("button", { name: "详情" }).click();
  await expect(page.getByText("测试用例详情 / 1 / 登录成功")).toBeVisible();
  await expect(page.getByText("输入账号")).toBeVisible();

  await page.getByRole("button", { name: "新增", exact: true }).click();
  const dialog = page.locator(".modal-card");
  await expect(dialog.getByText("新增测试用例")).toBeVisible();
  await dialog.getByPlaceholder("请输入用例名称").fill("新增用例");
  await dialog.getByRole("button", { name: "提交" }).click();
  await expect(page.getByText("项目/产品和用例名称不能为空。")).toBeVisible();
});
