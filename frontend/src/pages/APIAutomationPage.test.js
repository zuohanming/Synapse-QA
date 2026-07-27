import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({
  interfaces: { list: vi.fn(), get: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn() },
  requestHeaders: { list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn() },
  debug: { start: vi.fn(), get: vi.fn(), events: vi.fn(), stream: vi.fn(), cancel: vi.fn(), history: vi.fn(), historyDetail: vi.fn() },
  versions: { list: vi.fn(), get: vi.fn(), diff: vi.fn(), restore: vi.fn() }
}));
const configMock = vi.hoisted(() => ({
  projects: { list: vi.fn() },
  products: { list: vi.fn() },
  productModules: { list: vi.fn() },
  testObjects: { list: vi.fn() }
}));

vi.mock("../services/apiAutomationService.js", () => ({ apiAutomationService: apiMock }));
vi.mock("../services/configService.js", () => ({ configService: configMock }));

import { APIAutomationPage } from "./APIAutomationPage.js";

describe("接口自动化项目默认请求头", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("新增请求头时关联项目", async () => {
    configMock.projects.list.mockResolvedValue({ items: [{ id: 8, name: "商城项目" }] });
    apiMock.requestHeaders.list.mockResolvedValue({ items: [] });
    apiMock.requestHeaders.create.mockResolvedValue({});

    render(<APIAutomationPage activePath={["接口自动化", "请求头管理"]} />);
    await screen.findByText("项目默认请求头");
    fireEvent.click(screen.getByRole("button", { name: "新增请求头" }));
    fireEvent.change(screen.getAllByLabelText(/所属项目/)[1], { target: { value: "8" } });
    fireEvent.change(screen.getByPlaceholderText("例如 Authorization"), { target: { value: "Authorization" } });
    fireEvent.change(screen.getByPlaceholderText("例如 Bearer ${token}"), { target: { value: "Bearer demo" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));

    await waitFor(() => expect(apiMock.requestHeaders.create).toHaveBeenCalledWith(expect.objectContaining({
      projectId: 8,
      name: "Authorization",
      value: "Bearer demo"
    })));
  });

  it("接口执行时合并项目默认请求头且接口配置优先", async () => {
    configMock.products.list.mockResolvedValue({ items: [{ id: 12, projectId: 8, projectName: "商城项目", name: "Web端" }] });
    configMock.productModules.list.mockResolvedValue({ items: [] });
    configMock.testObjects.list.mockResolvedValue({ items: [] });
    apiMock.interfaces.list.mockResolvedValue({ items: [{
      id: 1,
      name: "查询商品",
      projectId: 8,
      projectName: "商城项目",
      productId: 12,
      productName: "Web端",
      moduleName: "商品",
      path: "https://example.com/products",
      method: "GET",
      protocol: "HTTPS",
      endpointType: "WEB",
      lifecycleStatus: "active",
      configuration: { headers: "{\"Authorization\":\"Bearer interface\"}" },
      revision: 1,
      updatedAt: "2026-07-24T10:00:00Z"
    }] });
    apiMock.requestHeaders.list.mockResolvedValue({ items: [
      { id: 1, projectId: 8, name: "Authorization", value: "Bearer project", enabled: true },
      { id: 2, projectId: 8, name: "X-Project", value: "mall", enabled: true }
    ] });
    apiMock.debug.start.mockResolvedValue({ taskId: "api-debug-1", executorId: "exec-api", status: "running" });
    apiMock.debug.history.mockResolvedValue([]);
    apiMock.versions.list.mockResolvedValue([]);
    apiMock.debug.stream.mockImplementation(async (_taskId, _after, onEvent) => {
      onEvent({ sequence: 2, progress: 100, message: "响应接收完成", status: "success" });
    });
    apiMock.debug.get.mockResolvedValue({
      status: "success",
      request: { method: "GET", url: "https://example.com/products" },
      result: { output: JSON.stringify({ statusCode: 200, headers: {}, body: "{}", durationMs: 25 }) }
    });
    apiMock.interfaces.get.mockImplementation(async () => {
      const payload = apiMock.interfaces.update.mock.calls.at(-1)?.[1] || {};
      return { ...payload, revision: Number(payload.revision || 0) + 1 };
    });

    render(<APIAutomationPage activePath={["接口自动化", "接口管理"]} />);
    await screen.findByText("查询商品");
    fireEvent.click(screen.getByRole("button", { name: "调试" }));
    expect(await screen.findByText("已加载 2 个项目默认请求头；接口内同名请求头优先。")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /后置 JSONPath 提取/ }));
    fireEvent.click(screen.getByRole("button", { name: "新增第一条规则" }));
    fireEvent.change(screen.getByPlaceholderText("例如 access_token"), { target: { value: "product_id" } });
    fireEvent.change(screen.getByPlaceholderText("$.data.token"), { target: { value: "$.data.id" } });
    fireEvent.click(screen.getByRole("button", { name: "保存 JSONPath 提取" }));
    await waitFor(() => expect(apiMock.interfaces.update).toHaveBeenCalledWith(1, expect.objectContaining({
      revision: 1,
      configuration: expect.objectContaining({
        jsonpath: expect.stringContaining('"name": "product_id"')
      })
    })));
    expect(await screen.findByText("接口配置已保存。")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("$.data.token"), { target: { value: "$.data.productId" } });
    fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() => expect(apiMock.interfaces.update).toHaveBeenLastCalledWith(1, expect.objectContaining({
      revision: 2,
      configuration: expect.objectContaining({
        jsonpath: expect.stringContaining("$.data.productId")
      })
    })));
    fireEvent.change(screen.getByLabelText("请求 URL"), { target: { value: "https://example.com/products?page=${page_no}" } });
    fireEvent.click(screen.getByRole("button", { name: /参数/ }));
    expect(screen.getByLabelText("参数名 1")).toHaveValue("page");
    expect(screen.getByLabelText("参数值 1")).toHaveValue("${page_no}");
    fireEvent.change(screen.getByLabelText("参数值 1"), { target: { value: "2" } });
    expect(screen.getByLabelText("请求 URL")).toHaveValue("https://example.com/products?page=2");
    fireEvent.change(screen.getByLabelText("参数值 1"), { target: { value: "${page_no}" } });
    expect(screen.getByLabelText("请求 URL")).toHaveValue("https://example.com/products?page=${page_no}");
    fireEvent.change(screen.getByLabelText("参数说明 1"), { target: { value: "页码" } });
    fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() => expect(apiMock.interfaces.update).toHaveBeenLastCalledWith(1, expect.objectContaining({
      revision: 3,
      configuration: expect.objectContaining({
        params: expect.stringContaining('"page": "${page_no}"'),
        paramsMeta: [expect.objectContaining({ key: "page", value: "${page_no}", description: "页码", enabled: true })]
      })
    })));
    fireEvent.change(screen.getByLabelText("请求 URL"), { target: { value: "https://example.com/products" } });
    await waitFor(() => {
      expect(screen.getByLabelText("参数名 1")).toHaveValue("");
      expect(screen.getByText("共 0 个参数，已启用 0 个")).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText("参数名 1"), { target: { value: "temporary" } });
    expect(screen.getByText("共 1 个参数，已启用 1 个")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("参数名 1"), { target: { value: "" } });
    expect(screen.getByText("共 0 个参数，已启用 0 个")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: /执行/ })[0]);

    await waitFor(() => expect(apiMock.debug.start).toHaveBeenCalled());
    expect(await screen.findByText("执行器调试完成。")).toBeInTheDocument();
    expect(apiMock.requestHeaders.list).toHaveBeenCalledWith({ projectId: "8" });
  });
});
