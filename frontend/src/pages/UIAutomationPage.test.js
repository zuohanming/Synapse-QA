import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TestCasesPage } from "./TestCasesPage.js";

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
    if (target.includes("/api/test-cases/export")) {
      if (globalThis.__exportFails) {
        return apiError("导出失败");
      }
      return apiResponse({ items: [{ id: 1, name: "登录成功" }], total: 1 });
    }
    if (target.includes("/api/test-cases/import")) {
      return apiResponse({ count: 1, message: "测试用例已导入" });
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
        productName: "演示DEMO/模拟UI",
        moduleName: "登录",
        pageName: "登录页",
        name: "登录成功",
        priority: "P1",
        status: "active",
        owner: "admin",
        steps: [{ id: 1, stepId: 40, stepName: "输入账号" }],
        datasets: [{ id: 1, name: "默认数据", variables: { username: "admin" }, enabled: true }]
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
            productId: 10,
            moduleId: 20,
            pageId: 30,
            productName: "演示DEMO/模拟UI",
            moduleName: "登录",
            pageName: "登录页",
            name: "登录成功",
            priority: "P1",
            status: globalThis.__caseStatus || "active",
            owner: globalThis.__missingOwner ? "" : "admin",
            steps: [{ stepId: 40, stepName: "输入账号" }],
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
    delete globalThis.__listFails;
    delete globalThis.__detailFails;
    delete globalThis.__exportFails;
    delete globalThis.__saveFails;
    delete globalThis.__caseDeleteFails;
    vi.restoreAllMocks();
  });

  it("渲染测试用例列表并调用专用 API", async () => {
    render(<TestCasesPage />);
    expect(await screen.findByText("登录成功")).toBeInTheDocument();
    expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases?page=1&pageSize=20"), expect.any(Object));
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
    const modal = document.querySelector(".modal-card");
    const selects = within(modal).getAllByRole("combobox");
    fireEvent.change(selects[1], { target: { value: "20" } });
    fireEvent.change(selects[2], { target: { value: "30" } });
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
    expect(await within(modal).findByText("输入账号")).toBeInTheDocument();
    fireEvent.click(within(modal).getByText("输入账号"));
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
    fireEvent.click(screen.getByText("提交"));
    expect(await screen.findByText("保存失败")).toBeInTheDocument();
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
    fireEvent.change(screen.getByLabelText("导入"), { target: { files: [file] } });
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining("/api/test-cases/import"), expect.objectContaining({ method: "POST" }));
    });
    const invalidFile = new File([JSON.stringify({ item: [] })], "invalid.json", { type: "application/json" });
    fireEvent.change(screen.getByLabelText("导入"), { target: { files: [invalidFile] } });
    expect(await screen.findByText("导入文件必须是数组或包含 items 数组")).toBeInTheDocument();
  });
});
