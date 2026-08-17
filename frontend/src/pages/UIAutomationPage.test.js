import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TestCasesPage } from "./TestCasesPage.js";
import { beautifyFlowNodes, buildDraftFlowNode, buildFlowConnectionPath, canConnectToTarget, StepWorkbench, UIAutomationPage } from "./UIAutomationPage.js";

function apiResponse(data) {
  return Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ data })
  });
}

function apiError(error) {
  return Promise.resolve({
    ok: false,
    json: () => Promise.resolve({ error })
  });
}

function mockFetch() {
  global.fetch = vi.fn((url, options = {}) => {
    const target = String(url);
    if (target.includes("/api/executors")) {
      return apiResponse(globalThis.__executorOffline ? [] : [{ executorId: "local", status: "online", supportedTypes: ["ui"] }]);
    }
    if (target.includes("/api/executions") && options.method === "POST") {
      return apiResponse({ id: 88, status: "running" });
    }
    if (target.includes("/api/test-cases/export")) {
      if (globalThis.__exportFails) {
        return apiError("导出失败");
      }
      return apiResponse({ items: [{ id: 1, name: "登录成功" }], total: 1 });
    }
    if (target.includes("/api/test-cases/import")) {
      return apiResponse(globalThis.__importNoCount ? { message: "测试用例已导入" } : { count: 1, message: "测试用例已导入" });
    }
    if (target.includes("/api/test-cases/1/datasets") && options.method === "POST") {
      return apiResponse({ message: "参数化数据已创建" });
    }
    if (target.includes("/api/test-cases/1/datasets/1") && options.method === "DELETE") {
      if (globalThis.__datasetDeleteFails) {
        return apiError("删除参数化数据失败");
      }
      return apiResponse({ message: "参数化数据已删除" });
    }
    if (target.includes("/api/test-cases/1") && !options.method) {
      if (globalThis.__detailFails) {
        return apiError("读取详情失败");
      }
      return apiResponse({
        id: 1,
        productId: 10,
        moduleId: 20,
        pageId: 30,
        productName: globalThis.__sparseDetail ? "" : "演示DEMO/模拟UI",
        moduleName: globalThis.__sparseDetail ? "" : "登录",
        pageName: globalThis.__sparseDetail ? "" : "登录页",
        name: globalThis.__sparseDetail ? "" : "登录成功",
        priority: "P1",
        status: globalThis.__sparseDetail ? "" : "active",
        owner: globalThis.__sparseDetail ? "" : "admin",
        steps: globalThis.__emptyDetailSteps ? [] : [{ id: 1, stepId: 40, stepName: globalThis.__sparseDetail ? "" : "输入账号" }],
        datasets: globalThis.__sparseDetail ? undefined : [{ id: 1, name: "默认数据", variables: { username: "admin" }, enabled: true }]
      });
    }
    if (target.includes("/api/test-cases/1") && options.method === "DELETE") {
      if (globalThis.__caseDeleteFails) {
        return apiError("删除失败");
      }
      return apiResponse({ message: "测试用例已删除" });
    }
    if (target.includes("/api/test-cases") && options.method === "POST") {
      if (globalThis.__saveFails) {
        return apiError("保存失败");
      }
      return apiResponse({ id: 2, message: "测试用例已创建" });
    }
    if (target.includes("/api/test-cases/1") && options.method === "PATCH") {
      if (globalThis.__saveFails) {
        return apiError("保存失败");
      }
      return apiResponse({ id: 1, message: "测试用例已更新" });
    }
    if (target.includes("/api/test-cases")) {
      if (globalThis.__listFails) {
        return apiError("列表失败");
      }
      return apiResponse({
        items: globalThis.__emptyCases
          ? []
          : [
          {
            id: 1,
            productId: globalThis.__sparseCase ? 0 : 10,
            moduleId: globalThis.__sparseCase ? 0 : 20,
            pageId: globalThis.__sparseCase ? 0 : 30,
            productName: globalThis.__missingRowNames ? "" : "演示DEMO/模拟UI",
            moduleName: globalThis.__missingRowNames ? "" : "登录",
            pageName: globalThis.__missingRowNames ? "" : "登录页",
            name: globalThis.__sparseCase ? "" : "登录成功",
            priority: "P1",
            status: globalThis.__caseStatus || "active",
            owner: globalThis.__missingOwner ? "" : "admin",
            steps: globalThis.__sparseCase ? undefined : [{ stepId: 40, stepName: "输入账号" }],
            updatedAt: "2026-06-17T10:00:00Z"
          }
        ],
        total: globalThis.__caseTotal || 1,
        page: 1,
        pageSize: 20
      });
    }
    if (target.includes("/api/config/products")) {
      return apiResponse({ items: [{ id: 10, projectName: "演示DEMO", name: "模拟UI" }], total: 1 });
    }
    if (target.includes("/api/config/product-modules")) {
      return apiResponse({ items: [{ id: 20, name: "登录" }], total: 1 });
    }
    if (target.includes("/api/ui/elements")) {
      return apiResponse({ items: [{ id: 30, name: "登录页" }], total: 1 });
    }
    if (target.includes("/api/ui/steps")) {
      return apiResponse({ items: [{ id: 40, name: "输入账号" }], total: 1 });
    }
    return apiResponse({});
  });
}

describe("UIAutomationPage 测试用例页", () => {
  beforeEach(() => {
    mockFetch();
    vi.stubGlobal("confirm", vi.fn(() => true));
    vi.stubGlobal("URL", {
      createObjectURL: vi.fn(() => "blob:test"),
      revokeObjectURL: vi.fn()
    });
  });

  afterEach(() => {
    cleanup();
    delete globalThis.__caseTotal;
    delete globalThis.__datasetDeleteFails;
    delete globalThis.__emptyCases;
    delete globalThis.__caseStatus;
    delete globalThis.__missingOwner;
    delete globalThis.__missingRowNames;
    delete globalThis.__sparseCase;
    delete globalThis.__sparseDetail;
    delete globalThis.__emptyDetailSteps;
    delete globalThis.__importNoCount;
    delete globalThis.__listFails;
    delete globalThis.__detailFails;
    delete globalThis.__exportFails;
    delete globalThis.__saveFails;
    delete globalThis.__caseDeleteFails;
    delete globalThis.__executorOffline;
    vi.restoreAllMocks();
  });

  it("渲染测试用例列表并调用专用 API", async () => {
    render(<TestCasesPage />);
    expect(await screen.findByText("登录成功")).toBeInTheDocument();
    expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases?page=1&pageSize=20"), expect.any(Object));
  });

  it("页面元素自动采集弹窗展示数组响应中的在线 UI 执行器", async () => {
    render(<UIAutomationPage activePath={["界面自动化", "页面元素"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "添加元素" }));
    fireEvent.click(await screen.findByRole("button", { name: "自动采集" }));
    expect(await screen.findByText("local")).toBeInTheDocument();
  });

  it("支持直接执行单条测试用例", async () => {
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByRole("button", { name: "执行用例 登录成功" }));
    expect(screen.getByRole("region", { name: "选择执行模式" })).toBeInTheDocument();
    fireEvent.click(screen.getByText("开始执行"));
    expect(await screen.findByText("执行批次 #88 已创建，请前往执行中心查看结果。")).toBeInTheDocument();
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/executions"),
      expect.objectContaining({ method: "POST", body: JSON.stringify({ runType: "ui", caseIds: [1], headless: true }) })
    );
  });

  it("支持选择有头模式执行", async () => {
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByRole("button", { name: "执行用例 登录成功" }));
    fireEvent.click(screen.getByText("有头执行"));
    fireEvent.click(screen.getByText("开始执行"));
    await screen.findByText("执行批次 #88 已创建，请前往执行中心查看结果。");
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/executions"),
      expect.objectContaining({ method: "POST", body: JSON.stringify({ runType: "ui", caseIds: [1], headless: false }) })
    );
  });

  it("执行器未启动时阻止执行并显示 Toast", async () => {
    globalThis.__executorOffline = true;
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByRole("button", { name: "执行用例 登录成功" }));
    fireEvent.click(screen.getByText("开始执行"));

    expect(await screen.findByRole("alert")).toHaveTextContent("执行器未启动，请先启动执行器");
    expect(global.fetch).not.toHaveBeenCalledWith(
      expect.stringContaining("/api/executions"),
      expect.objectContaining({ method: "POST" })
    );
  });

  it("用例没有关联步骤时阻止创建执行批次", async () => {
    globalThis.__emptyDetailSteps = true;
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByRole("button", { name: "执行用例 登录成功" }));
    fireEvent.click(screen.getByText("开始执行"));

    expect(await screen.findByRole("alert")).toHaveTextContent("以下用例未关联步骤：登录成功");
    expect(global.fetch).not.toHaveBeenCalledWith(
      expect.stringContaining("/api/executions"),
      expect.objectContaining({ method: "POST" })
    );
  });

  it("草稿用例允许执行，停用用例禁止执行", async () => {
    globalThis.__caseStatus = "draft";
    const draft = render(<TestCasesPage />);
    expect(await screen.findByRole("button", { name: "执行用例 登录成功" })).toBeEnabled();
    draft.unmount();

    globalThis.__caseStatus = "disabled";
    render(<TestCasesPage />);
    expect(await screen.findByRole("button", { name: "执行用例 登录成功" })).toBeDisabled();
  });

  it("渲染空列表、列表错误和未知状态", async () => {
    globalThis.__emptyCases = true;
    const { unmount } = render(<TestCasesPage />);
    expect(await screen.findByText("暂无数据")).toBeInTheDocument();
    unmount();

    globalThis.__emptyCases = false;
    globalThis.__listFails = true;
    const failed = render(<TestCasesPage />);
    expect(await screen.findByText("列表失败")).toBeInTheDocument();
    failed.unmount();

    globalThis.__listFails = false;
    globalThis.__caseStatus = "paused";
    globalThis.__missingOwner = true;
    render(<TestCasesPage />);
    expect(await screen.findByText("paused")).toBeInTheDocument();
    expect(screen.getAllByText("-").length).toBeGreaterThan(0);
  });

  it("渲染表格兜底字段和停用状态", async () => {
    globalThis.__missingRowNames = true;
    globalThis.__missingOwner = true;
    globalThis.__caseStatus = "disabled";
    render(<TestCasesPage />);
    expect(await screen.findByText("停用")).toBeInTheDocument();
    expect(screen.getAllByText("-").length).toBeGreaterThanOrEqual(4);
  });

  it("按名称筛选测试用例", async () => {
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.change(screen.getAllByPlaceholderText("请输入用例名称")[0], { target: { value: "登录" } });
    fireEvent.click(screen.getByText("搜索"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("name=%E7%99%BB%E5%BD%95"), expect.any(Object));
    });
  });

  it("重置筛选并切换分页大小", async () => {
    globalThis.__caseTotal = 40;
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.change(screen.getAllByPlaceholderText("请输入用例名称")[0], { target: { value: "登录" } });
    fireEvent.click(screen.getByText("重置"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases?page=1&pageSize=20"), expect.any(Object));
    });
    fireEvent.change(screen.getByDisplayValue("20 条/页"), { target: { value: "50" } });
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("pageSize=50"), expect.any(Object));
    });
  });

  it("筛选联动产品模块页面并切换全选状态", async () => {
    const { container } = render(<TestCasesPage />);
    await screen.findByText("登录成功");
    const selects = container.querySelectorAll("form.filter-grid select");
    fireEvent.change(selects[0], { target: { value: "10" } });
    await screen.findByText("登录");
    fireEvent.change(selects[1], { target: { value: "20" } });
    await screen.findAllByText("登录页");
    fireEvent.change(selects[2], { target: { value: "30" } });
    fireEvent.change(selects[3], { target: { value: "P1" } });
    fireEvent.change(selects[4], { target: { value: "active" } });
    const checkboxes = container.querySelectorAll('input[type="checkbox"]');
    fireEvent.click(checkboxes[0]);
    expect(checkboxes[1]).toBeChecked();
    fireEvent.click(checkboxes[1]);
    expect(checkboxes[0]).not.toBeChecked();
    fireEvent.click(checkboxes[1]);
    fireEvent.click(checkboxes[0]);
    expect(checkboxes[1]).not.toBeChecked();
    fireEvent.click(screen.getByText("搜索"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("productId=10"), expect.any(Object));
    });
  });

  it("新增表单校验必填项", async () => {
    render(<TestCasesPage />);
    await screen.findAllByText("登录成功");
    fireEvent.click(screen.getByText("新增"));
    fireEvent.click(screen.getByText("提交"));
    expect(await screen.findByText("项目/产品和用例名称不能为空。")).toBeInTheDocument();
  });

  it("新增测试用例成功并保存文本配置", async () => {
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("新增"));
    const modal = document.querySelector(".modal-card");
    await within(modal).findByText("演示DEMO/模拟UI");
    let selects = within(modal).getAllByRole("combobox");
    fireEvent.change(selects[0], { target: { value: "10" } });
    await within(modal).findByText("登录");
    selects = within(modal).getAllByRole("combobox");
    fireEvent.change(selects[1], { target: { value: "20" } });
    await within(modal).findByText("登录页");
    selects = within(modal).getAllByRole("combobox");
    fireEvent.change(selects[2], { target: { value: "30" } });
    fireEvent.change(selects[3], { target: { value: "mixed" } });
    fireEvent.change(selects[4], { target: { value: "P1" } });
    fireEvent.change(selects[5], { target: { value: "active" } });
    fireEvent.change(within(modal).getByPlaceholderText("请输入用例名称"), { target: { value: "新增用例" } });
    fireEvent.change(within(modal).getByPlaceholderText("请输入负责人"), { target: { value: "qa" } });
    const textareas = modal.querySelectorAll("textarea");
    fireEvent.change(textareas[0], { target: { value: "准备账号" } });
    fireEvent.change(textareas[1], { target: { value: "看到首页" } });
    fireEvent.change(textareas[2], { target: { value: "新增说明" } });
    fireEvent.click(screen.getByText("提交"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases"), expect.objectContaining({ method: "POST" }));
    });
  });

  it("导出测试用例", async () => {
    const click = vi.fn();
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    const originalCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tagName, options) => {
      if (tagName === "a") {
        return { click, set href(value) {}, set download(value) {} };
      }
      return originalCreateElement(tagName, options);
    });
    fireEvent.click(screen.getByText("导出"));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases/export"), expect.any(Object));
  });

  it("处理导出失败和详情读取失败", async () => {
    globalThis.__exportFails = true;
    const exported = render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("导出"));
    expect(await screen.findByText("导出失败")).toBeInTheDocument();
    exported.unmount();

    globalThis.__exportFails = false;
    globalThis.__detailFails = true;
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("详情"));
    expect(await screen.findByText("读取详情失败")).toBeInTheDocument();
  });

  it("编辑测试用例成功", async () => {
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("编辑"));
    await screen.findByText("编辑测试用例");
    const modal = document.querySelector(".modal-card");
    const selects = within(modal).getAllByRole("combobox");
    expect(await within(modal).findByText("输入账号")).toBeInTheDocument();
    fireEvent.change(selects[3], { target: { value: "api" } });
    fireEvent.change(selects[4], { target: { value: "P0" } });
    fireEvent.change(selects[5], { target: { value: "disabled" } });
    fireEvent.change(within(modal).getByPlaceholderText("请输入用例名称"), { target: { value: "编辑用例" } });
    fireEvent.change(within(modal).getByPlaceholderText("请输入负责人"), { target: { value: "tester" } });
    fireEvent.change(within(modal).getByPlaceholderText("smoke,login"), { target: { value: "smoke" } });
    fireEvent.click(within(modal).getByText("启用参数化"));
    const textareas = within(modal).getAllByRole("textbox");
    fireEvent.change(textareas[0], { target: { value: "存在账号" } });
    fireEvent.change(textareas[1], { target: { value: "进入首页" } });
    fireEvent.change(textareas[2], { target: { value: "编辑说明" } });
    expect(within(modal).getByRole("checkbox", { name: "输入账号" })).toBeChecked();
    fireEvent.click(screen.getByText("提交"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/test-cases/1"),
        expect.objectContaining({ method: "PATCH" })
      );
    });
  });

  it("编辑测试用例保存失败", async () => {
    globalThis.__saveFails = true;
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("编辑"));
    await screen.findByText("编辑测试用例");
    await screen.findByText("输入账号");
    fireEvent.click(screen.getByText("提交"));
    expect(await screen.findByText("保存失败")).toBeInTheDocument();
  });

  it("编辑稀疏测试用例时使用默认表单值", async () => {
    globalThis.__sparseCase = true;
    render(<TestCasesPage />);
    await screen.findByText("P1");
    fireEvent.click(screen.getByText("编辑"));
    await screen.findByText("编辑测试用例");
    const modal = document.querySelector(".modal-card");
    const selects = within(modal).getAllByRole("combobox");
    expect(selects[3]).toHaveValue("ui");
    expect(selects[4]).toHaveValue("P1");
    fireEvent.click(within(modal).getByText("取消"));
    await waitFor(() => {
      expect(document.querySelector(".modal-card")).not.toBeInTheDocument();
    });
  });

  it("查看详情并维护参数化数据", async () => {
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("详情"));
    expect(await screen.findByText("测试用例详情 / 1 / 登录成功")).toBeInTheDocument();
    expect(screen.getByText("输入账号")).toBeInTheDocument();
    const detailPanel = document.querySelector(".detail-panel");
    fireEvent.change(screen.getByPlaceholderText("数据集名称"), { target: { value: "新数据" } });
    fireEvent.click(within(detailPanel).getByText("启用"));
    fireEvent.change(screen.getByPlaceholderText('{"username":"admin"}'), { target: { value: "[" } });
    fireEvent.click(screen.getByText("新增数据集"));
    expect(await screen.findByText(/Unexpected end of JSON input|保存参数化数据失败/)).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText('{"username":"admin"}'), { target: { value: '{"username":"test"}' } });
    fireEvent.click(screen.getByText("新增数据集"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases/1/datasets"), expect.objectContaining({ method: "POST" }));
    });
    global.confirm.mockReturnValueOnce(false);
    fireEvent.click(within(detailPanel).getByText("删除"));
    expect(global.confirm).toHaveBeenCalled();
    fireEvent.click(within(detailPanel).getByText("删除"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases/1/datasets/1"), expect.objectContaining({ method: "DELETE" }));
    });
    fireEvent.click(within(detailPanel).getByText("关闭"));
    await waitFor(() => {
      expect(screen.queryByText("测试用例详情 / 1 / 登录成功")).not.toBeInTheDocument();
    });
  });

  it("详情页显示空字段、空步骤和步骤 ID 兜底", async () => {
    globalThis.__sparseDetail = true;
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("详情"));
    expect(await screen.findByText("40")).toBeInTheDocument();
    expect(screen.getAllByText(/：-/).length).toBeGreaterThan(0);
    cleanup();

    globalThis.__sparseDetail = false;
    globalThis.__emptyDetailSteps = true;
    mockFetch();
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("详情"));
    expect(await screen.findByText("暂无关联步骤")).toBeInTheDocument();
  });

  it("批量删除并处理参数化数据删除失败", async () => {
    const { container } = render(<TestCasesPage />);
    await screen.findByText("登录成功");
    fireEvent.click(screen.getByText("批量删除"));
    expect(await screen.findByText("请先选择需要删除的测试用例。")).toBeInTheDocument();
    const checkboxes = container.querySelectorAll('input[type="checkbox"]');
    fireEvent.click(checkboxes[1]);
    fireEvent.click(screen.getByText("批量删除"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases/1"), expect.objectContaining({ method: "DELETE" }));
    });
    fireEvent.click(screen.getByText("详情"));
    await screen.findByText("测试用例详情 / 1 / 登录成功");
    globalThis.__datasetDeleteFails = true;
    fireEvent.click(within(document.querySelector(".detail-panel")).getByText("删除"));
    expect(await screen.findByText("删除参数化数据失败")).toBeInTheDocument();
  });

  it("删除和导入测试用例", async () => {
    render(<TestCasesPage />);
    await screen.findByText("登录成功");
    global.confirm.mockReturnValueOnce(false);
    fireEvent.click(screen.getByText("删除"));
    expect(global.confirm).toHaveBeenCalled();
    globalThis.__caseDeleteFails = true;
    fireEvent.click(screen.getByText("删除"));
    expect(await screen.findByText("删除失败")).toBeInTheDocument();
    globalThis.__caseDeleteFails = false;
    fireEvent.click(screen.getByText("删除"));
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases/1"), expect.objectContaining({ method: "DELETE" }));
    });
    const file = new File([JSON.stringify([{ productId: 10, name: "导入用例" }])], "cases.json", { type: "application/json" });
    globalThis.__importNoCount = true;
    fireEvent.change(screen.getByLabelText("导入"), { target: { files: [file] } });
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases/import"), expect.objectContaining({ method: "POST" }));
    });
    expect(await screen.findByText("已导入 1 条测试用例。")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("导入"), { target: { files: [] } });
    const invalidFile = new File([JSON.stringify({ item: [] })], "invalid.json", { type: "application/json" });
    fireEvent.change(screen.getByLabelText("导入"), { target: { files: [invalidFile] } });
    expect(await screen.findByText("导入文件必须是数组或包含 items 数组")).toBeInTheDocument();
  });
});

describe("画布自动布局", () => {
  const nodesDoNotOverlap = (nodes) => nodes.every((node, index) => nodes.slice(index + 1).every((other) => (
    node.x + 156 <= other.x || other.x + 156 <= node.x || node.y + 64 <= other.y || other.y + 64 <= node.y
  )));

  it("将多个未连接节点放入自适应矩阵，单个节点保持区域居中", () => {
    const looseNodes = Array.from({ length: 5 }, (_, index) => ({ id: index + 1, type: "元素操作" }));
    const matrix = beautifyFlowNodes(looseNodes, [], { width: 1200, height: 720 });
    const xs = new Set(matrix.nodes.map((node) => node.x));
    expect(xs.size).toBeGreaterThan(1);
    expect(nodesDoNotOverlap(matrix.nodes)).toBe(true);

    const single = beautifyFlowNodes([{ id: 1, type: "元素操作" }], [], { width: 1200, height: 720 });
    expect(single.nodes[0].x).toBe(24);
    expect(single.nodes[0].y).toBe(80);
  });

  it("将连续线性流程按稳定顺序填充为多列矩阵", () => {
    const nodes = Array.from({ length: 9 }, (_, index) => ({ id: index + 1, type: "元素操作" }));
    const connections = nodes.slice(1).map((node, index) => ({ from: index + 1, to: node.id }));
    const result = beautifyFlowNodes(nodes, connections, { width: 1200, height: 720 });
    expect(new Set(result.nodes.map((node) => node.x)).size).toBeGreaterThanOrEqual(2);
    expect(new Set(result.nodes.map((node) => node.y)).size).toBeGreaterThanOrEqual(2);
    expect(nodesDoNotOverlap(result.nodes)).toBe(true);
    expect(result.nodes.map((node) => node.id)).toEqual(nodes.map((node) => node.id));
    expect(result.nodes[0].x).toBe(24);
    expect(result.nodes[0].y).toBe(24);
    expect(result.nodes[5].y - result.nodes[0].y).toBeGreaterThanOrEqual(64 + 80);
    const wider = beautifyFlowNodes(nodes, connections, { width: 1800, height: 1000 });
    expect(wider.nodes[0].x).toBe(24);
    expect(wider.nodes[0].y).toBe(24);
    const secondRow = result.nodes.filter((node) => node.y === result.nodes[5].y);
    expect(secondRow.map((node) => node.x)).toEqual([...secondRow.map((node) => node.x)].sort((a, b) => b - a));
    const crossRowPath = buildFlowConnectionPath(result.nodes[4], result.nodes[5], { from: 5, to: 6 });
    expect(crossRowPath).toContain("Q");
    expect(crossRowPath).toContain("V");
  });

  it("按空间选择方向，整理条件分支和未连接节点且不改变输入", () => {
    const nodes = [
      { id: 1, type: "条件判断", x: 400, y: 400 },
      { id: 2, type: "元素操作", x: 500, y: 400 },
      { id: 3, type: "元素操作", x: 600, y: 400 },
      { id: 4, type: "元素操作", x: 700, y: 400 }
    ];
    const connections = [{ from: 1, to: 2, branch: "true" }, { from: 1, to: 3, branch: "false" }];
    const original = JSON.parse(JSON.stringify(nodes));
    const horizontal = beautifyFlowNodes(nodes, connections, { width: 1200, height: 720 });
    const vertical = beautifyFlowNodes(nodes, connections, { width: 350, height: 300 });
    expect(horizontal.direction).toBe("horizontal");
    expect(vertical.direction).toBe("vertical");
    expect(horizontal.nodes).toHaveLength(nodes.length);
    const horizontalPositions = horizontal.nodes.map((node) => ({ x: node.x, y: node.y }));
    const verticalPositions = vertical.nodes.map((node) => ({ x: node.x, y: node.y }));
    expect(horizontal.nodes.find((node) => node.id === 4).y).toBeGreaterThan(horizontal.nodes.find((node) => node.id === 2).y);
    expect(Math.min(...horizontalPositions.map((position) => position.x))).toBeGreaterThan(0);
    expect(Math.min(...horizontalPositions.map((position) => position.y))).toBeGreaterThan(0);
    [horizontalPositions, verticalPositions].forEach((positions) => {
      positions.forEach((position, index) => positions.slice(index + 1).forEach((other) => {
        expect(position.x + 156 <= other.x || other.x + 156 <= position.x || position.y + 64 <= other.y || other.y + 64 <= position.y).toBe(true);
      }));
    });
    expect(nodesDoNotOverlap(horizontal.nodes)).toBe(true);
    expect(nodes).toEqual(original);
    expect(connections).toEqual([{ from: 1, to: 2, branch: "true" }, { from: 1, to: 3, branch: "false" }]);
  });
});

describe("未保存画布节点", () => {
  it("创建后立即具有稳定 ID、位置并保持未保存状态", () => {
    const node = buildDraftFlowNode({ label: "元素操作", color: "#123456" }, "draft-7", 180, 120);
    expect(node).toMatchObject({ id: "draft-7", x: 180, y: 120, saved: false, type: "元素操作" });
  });

  it("新建草稿节点不自动改变既有连线", () => {
    const connections = [{ from: 1, to: 2 }];
    const draft = buildDraftFlowNode({ label: "元素操作", color: "#123456" }, 3, 180, 120);
    expect(connections).toEqual([{ from: 1, to: 2 }]);
    expect(draft.saved).toBe(false);
  });
});

describe("直接点击节点连线", () => {
  it("允许点击目标卡片完成连接并拒绝源节点", () => {
    expect(canConnectToTarget({ nodeId: 1, branch: "" }, { id: 2 })).toBe(true);
    expect(canConnectToTarget({ nodeId: 1, branch: "" }, { id: 1 })).toBe(false);
    expect(canConnectToTarget(null, { id: 2 })).toBe(false);
  });

  it("真实渲染中点击连线进入模式并点击目标节点完成连接", async () => {
    const step = {
      id: 40,
      name: "演示步骤",
      description: JSON.stringify({ schema: "synapse-flow-v1", nodes: [
        { id: 1, type: "元素操作", title: "起点", x: 60, y: 80, saved: true, values: {}, params: [] },
        { id: 2, type: "元素操作", title: "目标", x: 320, y: 80, saved: true, values: {}, params: [] }
      ], connections: [] })
    };
    render(<StepWorkbench step={step} onBack={vi.fn()} />);
    fireEvent.mouseDown(screen.getAllByRole("button", { name: /元素操作起点/ })[0]);
    fireEvent.mouseUp(screen.getAllByRole("button", { name: /元素操作起点/ })[0]);
    expect((await screen.findAllByText("已选择「起点」，请直接点击目标节点")).length).toBeGreaterThan(0);
    fireEvent.click(screen.getAllByRole("button", { name: /元素操作目标/ })[0]);
    expect(screen.getByText("连线 1")).toBeInTheDocument();
    expect(document.querySelectorAll(".flow-connection-path")).toHaveLength(1);
    fireEvent.click(document.querySelector(".flow-canvas"));
    expect(screen.queryByText("已选择「起点」，请直接点击目标节点")).not.toBeInTheDocument();
  });
});

describe("节点操作选择器", () => {
  const step = {
    id: 41,
    name: "操作选择步骤",
    description: JSON.stringify({ schema: "synapse-flow-v1", nodes: [
      { id: 1, type: "元素操作", title: "起点", x: 60, y: 80, saved: true, values: {}, params: [] }
    ], connections: [] })
  };

  beforeEach(() => cleanup());
  afterEach(() => cleanup());

  function selectNode() {
    const nodeEl = screen.getAllByText("起点").map((el) => el.closest(".flow-node")).find(Boolean);
    fireEvent.mouseDown(nodeEl);
    fireEvent.mouseUp(nodeEl);
  }

  it("点击触发弹出下拉面板，切换分类并选择操作回填", async () => {
    render(<StepWorkbench step={step} onBack={vi.fn()} />);
    selectNode();
    fireEvent.click(screen.getByRole("button", { name: /请选择节点操作/ }));
    const listbox = screen.getByRole("listbox", { name: /操作列表/ });
    expect(listbox).toBeInTheDocument();
    expect(within(listbox).getByText("强制等待")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("treeitem", { name: /元素操作/ }));
    expect(within(listbox).getByText("元素单击")).toBeInTheDocument();

    fireEvent.click(within(listbox).getByRole("option", { name: /元素单击/ }));
    expect(screen.getByRole("button", { name: /元素操作 \/ 元素单击/ })).toBeInTheDocument();
  });

  it("Enter 确认选中高亮项，ESC 关闭面板", async () => {
    render(<StepWorkbench step={step} onBack={vi.fn()} />);
    selectNode();
    fireEvent.click(screen.getByRole("button", { name: /请选择节点操作/ }));
    const listbox = screen.getByRole("listbox", { name: /操作列表/ });
    fireEvent.keyDown(listbox, { key: "Enter" });
    expect(screen.getByRole("button", { name: /浏览器操作 \/ 强制等待/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /浏览器操作 \/ 强制等待/ }));
    const panel = document.querySelector(".operation-dropdown-panel");
    fireEvent.keyDown(panel, { key: "Escape" });
    expect(document.querySelector(".operation-dropdown-panel")).toBeNull();
  });
});
