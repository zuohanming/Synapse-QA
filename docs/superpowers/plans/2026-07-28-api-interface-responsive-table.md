# 接口管理自适应搜索与列表 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让接口管理搜索区在桌面端保持单行，并让接口列表完整适配可用宽度、取消横向滚动，同时保留字段截断气泡。

**Architecture:** 为通用 `DataTable` 增加显式的 `fitContainer` 布局模式，默认行为保持不变；接口管理页单独启用该模式并声明百分比列宽。搜索区通过页面专属类设置弹性网格，避免影响请求头管理等共用筛选组件。

**Tech Stack:** React 19、Vitest、Testing Library、Vite、原生 CSS、Playwright。

## Global Constraints

- 仅修改前端布局与展示，不修改 API、筛选状态、分页、批量操作或数据结构。
- 1440×900、浏览器 100% 缩放时搜索区必须保持单行。
- 接口列表的 `scrollWidth` 不得大于 `clientWidth`。
- 小于 900px 时保留移动端纵向布局，不通过页面横向滚动强制单行。
- 超长字段继续使用现有 `TableOverflowTooltip` 气泡。
- 每次代码或文档变更同步更新 `docs/daily-updates/2026-07-28.md`；用户可见行为同步更新 `CHANGELOG.md`。

---

### Task 1: 为 DataTable 增加容器适配模式

**Files:**
- Create: `frontend/src/components/DataTable.test.js`
- Modify: `frontend/src/components/DataTable.js`

**Interfaces:**
- Consumes: `columns: Array<{ key, title, width?, render? }>`。
- Produces: `DataTable({ fitContainer?: boolean })`；启用时表格增加 `data-fit-container="true"`，列宽允许使用百分比字符串，不再计算像素最小总宽度。

- [ ] **Step 1: 写失败测试**

```jsx
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { DataTable } from "./DataTable.js";

describe("DataTable 容器适配模式", () => {
  afterEach(cleanup);

  it("使用百分比列宽且不设置超出容器的最小宽度", () => {
    const { container } = render(
      <DataTable
        fitContainer
        columns={[
          { key: "id", title: "ID", width: "20%" },
          { key: "name", title: "名称", width: "80%" }
        ]}
        rows={[{ id: 1, name: "登录接口" }]}
      />
    );

    const table = container.querySelector(".data-table");
    const columns = container.querySelectorAll("col");
    expect(table).toHaveAttribute("data-fit-container", "true");
    expect(table.style.minWidth).toBe("");
    expect(columns[0]).toHaveStyle({ width: "20%" });
    expect(columns[1]).toHaveStyle({ width: "80%" });
  });
});
```

- [ ] **Step 2: 运行测试并确认正确失败**

Run: `npm test -- src/components/DataTable.test.js`

Expected: FAIL，原因是表格没有 `data-fit-container="true"`，且仍存在像素 `minWidth`。

- [ ] **Step 3: 写最小实现**

在 `frontend/src/components/DataTable.js` 中加入：

```jsx
function columnWidth(column) {
  return column.width || defaultColumnWidths[column.key] || 160;
}

function cssWidth(value) {
  return typeof value === "number" ? `${value}px` : value;
}

export function DataTable({ columns, rows, rowKey = "id", emptyText = "暂无数据", fitContainer = false }) {
  const tableWidth = fitContainer
    ? undefined
    : columns.reduce((total, column) => total + Number(columnWidth(column)), 0);

  return (
    <div className={fitContainer ? "table-wrap table-wrap-fit" : "table-wrap"}>
      <table
        className="data-table"
        data-fit-container={fitContainer ? "true" : undefined}
        style={tableWidth ? { minWidth: `${tableWidth}px` } : undefined}
      >
        <colgroup>
          {columns.map((column) => (
            <col key={column.key} style={{ width: cssWidth(columnWidth(column)) }} />
          ))}
        </colgroup>
        {/* 保留现有 thead、tbody、气泡包装和空状态 */}
      </table>
    </div>
  );
}
```

实现时保留现有表头、数据行、`table-cell-content`、`data-overflow-tooltip` 和分页代码不变。

- [ ] **Step 4: 运行测试确认通过**

Run: `npm test -- src/components/DataTable.test.js`

Expected: PASS。

- [ ] **Step 5: 提交**

```powershell
git add -- frontend/src/components/DataTable.js frontend/src/components/DataTable.test.js
git commit -m "feat: add fitted data table layout"
```

---

### Task 2: 接口管理启用专属列宽和单行搜索区

**Files:**
- Modify: `frontend/src/pages/APIAutomationPage.js`
- Modify: `frontend/src/pages/APIAutomationPage.test.js`

**Interfaces:**
- Consumes: Task 1 的 `DataTable fitContainer`。
- Produces: `.api-interface-filter` 单行搜索表单；接口列表列宽百分比总和为 `100%`。

- [ ] **Step 1: 写失败测试**

在 `frontend/src/pages/APIAutomationPage.test.js` 的接口管理测试组中增加：

```jsx
it("接口管理使用单行搜索区和容器适配列表", async () => {
  configMock.projects.list.mockResolvedValue({ items: [] });
  configMock.products.list.mockResolvedValue({ items: [] });
  configMock.productModules.list.mockResolvedValue({ items: [] });
  apiMock.interfaces.list.mockResolvedValue({ items: [], total: 0 });

  const { container } = render(
    <APIAutomationPage activePath={["接口自动化", "接口管理"]} />
  );

  await screen.findByText("接口信息收集");
  expect(container.querySelector("form.api-interface-filter")).toBeInTheDocument();
  expect(container.querySelector(".api-interface-list .data-table")).toHaveAttribute("data-fit-container", "true");
  expect(
    [...container.querySelectorAll(".api-interface-list col")].map((column) => column.style.width)
  ).toEqual(["3%", "4%", "12%", "9%", "11%", "21%", "7%", "8%", "8%", "10%", "7%"]);
});
```

- [ ] **Step 2: 运行测试并确认正确失败**

Run: `npm test -- src/pages/APIAutomationPage.test.js`

Expected: FAIL，原因是表单缺少 `.api-interface-filter`，列表未启用 `fitContainer`，列宽仍为像素值。

- [ ] **Step 3: 写最小实现**

在接口管理列定义中加入宽度：

```jsx
const columns = [
  { key: "select", title: ..., width: "3%", render: ... },
  { key: "id", title: "ID", width: "4%" },
  { key: "productName", title: "项目/产品", width: "12%", render: ... },
  { key: "moduleName", title: "模块名称", width: "9%", render: ... },
  { key: "name", title: "接口名称", width: "11%" },
  { key: "path", title: "方法 / 路径", width: "21%", render: ... },
  { key: "endpointType", title: "端类型", width: "7%", render: ... },
  { key: "lifecycleStatus", title: "接口状态", width: "8%", render: ... },
  { key: "lastDebugStatus", title: "最近调试", width: "8%", render: ... },
  { key: "updatedBy", title: "最近修改", width: "10%", render: ... },
  { key: "operations", title: "操作", width: "7%", render: ... }
];
```

修改接口管理表单和表格调用：

```jsx
<form className="api-filter-grid api-interface-filter" ...>
```

```jsx
<DataTable fitContainer columns={columns} rows={pageRows} emptyText="暂无接口数据" />
```

- [ ] **Step 4: 运行测试确认通过**

Run: `npm test -- src/pages/APIAutomationPage.test.js`

Expected: PASS。

- [ ] **Step 5: 提交**

```powershell
git add -- frontend/src/pages/APIAutomationPage.js frontend/src/pages/APIAutomationPage.test.js
git commit -m "feat: fit interface management table to viewport"
```

---

### Task 3: 完成接口管理专属 CSS 与真实浏览器验收

**Files:**
- Modify: `frontend/src/styles/appearance.css`
- Modify: `docs/daily-updates/2026-07-28.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `.api-interface-filter`、`.table-wrap-fit`、`[data-fit-container="true"]`。
- Produces: 桌面单行筛选、无横向滚动列表、紧凑操作列和移动端降级。

- [ ] **Step 1: 写浏览器验收测试**

在现有 Playwright 浏览器检查中使用以下断言；若项目没有对应 E2E 文件，则先创建 `frontend/e2e/api-interface-layout.spec.js`：

```js
import { expect, test } from "@playwright/test";

test("接口管理在 100% 缩放下无列表横向滚动", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  // 使用现有登录辅助或 admin/admin123 登录，然后进入接口自动化 / 接口管理。

  const filter = page.locator(".api-interface-filter");
  const filterItems = filter.locator(":scope > label, :scope > .api-filter-actions");
  const tops = await filterItems.evaluateAll((items) => items.map((item) => Math.round(item.getBoundingClientRect().top)));
  expect(new Set(tops).size).toBe(1);

  const tableWrap = page.locator(".api-interface-list .table-wrap");
  const dimensions = await tableWrap.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
  await expect(page.getByRole("columnheader", { name: "最近修改" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "操作" })).toBeVisible();
});
```

- [ ] **Step 2: 运行浏览器测试并确认正确失败**

Run: `npm run e2e -- e2e/api-interface-layout.spec.js`

Expected: FAIL，搜索字段存在多个 `top` 值或表格 `scrollWidth > clientWidth`。

- [ ] **Step 3: 写最小 CSS**

追加到 `frontend/src/styles/appearance.css`：

```css
.api-interface-filter {
  display: grid;
  grid-template-columns:
    minmax(120px, 1.35fr)
    minmax(118px, 1.25fr)
    minmax(104px, 1fr)
    minmax(92px, .72fr)
    minmax(100px, .82fr)
    minmax(100px, .82fr)
    max-content;
  align-items: end;
  gap: 10px;
}

.api-interface-filter .api-filter-actions {
  flex-wrap: nowrap;
  white-space: nowrap;
}

.api-interface-list .table-wrap-fit {
  overflow-x: hidden;
}

.api-interface-list .data-table[data-fit-container="true"] {
  width: 100%;
  min-width: 0 !important;
}

.api-interface-list .data-table[data-fit-container="true"] th,
.api-interface-list .data-table[data-fit-container="true"] td {
  padding-inline: clamp(6px, .65vw, 12px);
}

.api-interface-list .action-links {
  display: flex;
  gap: 5px;
  white-space: nowrap;
}

.api-interface-list .action-links button {
  padding-inline: 2px;
}

@media (max-width: 899px) {
  .api-interface-filter {
    grid-template-columns: minmax(0, 1fr);
  }
}
```

- [ ] **Step 4: 运行页面测试和浏览器测试**

Run:

```powershell
npm test -- src/components/DataTable.test.js src/pages/APIAutomationPage.test.js
npm run e2e -- e2e/api-interface-layout.spec.js
```

Expected: 全部 PASS；1440×900 下搜索字段顶部位置相同，表格无横向溢出。

- [ ] **Step 5: 运行全量验证**

Run:

```powershell
npm test -- --run
npm run build
```

Expected: 39 项既有测试与新增测试全部通过；Vite 构建成功，仅允许保留项目既有的 500kB 分包体积提示。

- [ ] **Step 6: 更新项目记录**

在 `docs/daily-updates/2026-07-28.md` 追加“接口管理单行搜索与自适应列表”，包含变更内容、影响范围和实际验证结果；在 `CHANGELOG.md` 的 `Unreleased` 追加用户可见行为说明。

- [ ] **Step 7: 提交**

```powershell
git add -- frontend/src/styles/appearance.css frontend/e2e/api-interface-layout.spec.js docs/daily-updates/2026-07-28.md CHANGELOG.md
git commit -m "feat: optimize interface management layout"
```
