import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({
  interfaces: { list: vi.fn(), get: vi.fn(), create: vi.fn(), update: vi.fn(), saveConfiguration: vi.fn(), remove: vi.fn() },
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

import { APIAutomationPage, parseHeaderRows, parseTemporaryVariableRows } from "./APIAutomationPage.js";

describe("接口自动化项目默认请求头", () => {
  it("恢复接口中已保存的临时变量配置", () => {
    expect(parseTemporaryVariableRows({
      temporaryVariables: [{ key: "token", type: "string", value: "saved-token", description: "登录令牌", enabled: false }]
    })).toEqual([{ key: "token", type: "string", value: "saved-token", description: "登录令牌", enabled: false }]);
  });

  it("兼容历史请求头并恢复结构化元数据", () => {
    expect(parseHeaderRows({ headers: "{\"Authorization\":\"Bearer legacy\"}" })).toEqual([
      { key: "Authorization", value: "Bearer legacy", description: "", enabled: true }
    ]);
    expect(parseHeaderRows({
      headersMeta: [{ key: "X-Trace", value: "${trace_id}", description: "链路标识", enabled: false }]
    })).toEqual([
      { key: "X-Trace", value: "${trace_id}", description: "链路标识", enabled: false }
    ]);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("接口管理列表使用视口适配列宽", async () => {
    apiMock.interfaces.list.mockResolvedValue({ items: [], total: 0 });
    configMock.projects.list.mockResolvedValue({ items: [] });
    configMock.products.list.mockResolvedValue({ items: [] });
    configMock.productModules.list.mockResolvedValue({ items: [] });

    const { container } = render(<APIAutomationPage activePath={["接口自动化", "接口管理"]} />);

    await waitFor(() => expect(container.querySelector(".api-interface-list .data-table")).toBeInTheDocument());
    expect(container.querySelector("form.api-interface-filter")).toBeInTheDocument();
    const table = container.querySelector(".api-interface-list .data-table");
    expect(table).toHaveAttribute("data-fit-container", "true");
    expect(Array.from(table.querySelectorAll("col")).map((column) => column.style.width)).toEqual([
      "3%", "4%", "12%", "9%", "11%", "21%", "7%", "8%", "8%", "10%", "7%"
    ]);
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
      const payload = apiMock.interfaces.saveConfiguration.mock.calls.at(-1)?.[1] || {};
      return { configuration: payload.configuration || {}, revision: Number(payload.revision || 0) + 1 };
    });

    render(<APIAutomationPage activePath={["接口自动化", "接口管理"]} />);
    await screen.findByText("查询商品");
    fireEvent.click(screen.getByRole("button", { name: "调试" }));
    expect(await screen.findByText("请求头列表")).toBeInTheDocument();
    expect(screen.getByLabelText("接口请求头名称 1")).toHaveValue("Authorization");
    expect(screen.getByLabelText("接口请求头值 1")).toHaveValue("Bearer interface");
    expect(screen.getByLabelText("项目请求头 X-Project")).toBeDisabled();
    expect(screen.getByLabelText("自动请求头 Host")).toBeDisabled();
    expect(screen.getByText("自动生成 3")).toBeInTheDocument();
    expect(screen.getByText("项目默认 1")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("接口请求头名称 2"), { target: { value: "X-Debug" } });
    fireEvent.change(screen.getByLabelText("接口请求头值 2"), { target: { value: "${trace_id}" } });
    fireEvent.click(screen.getByRole("button", { name: /后置 JSONPath 提取/ }));
    fireEvent.click(screen.getByRole("button", { name: "新增第一条规则" }));
    fireEvent.change(screen.getByPlaceholderText("例如 access_token"), { target: { value: "product_id" } });
    fireEvent.change(screen.getByPlaceholderText("$.data.token"), { target: { value: "$.data.id" } });
    fireEvent.click(screen.getByRole("button", { name: "保存 JSONPath 提取" }));
    await waitFor(() => expect(apiMock.interfaces.saveConfiguration).toHaveBeenCalledWith(1, expect.objectContaining({
      revision: 1,
      configuration: expect.objectContaining({
        headers: expect.stringContaining('"X-Debug": "${trace_id}"'),
        headersMeta: expect.arrayContaining([
          expect.objectContaining({ key: "Authorization", value: "Bearer interface", enabled: true }),
          expect.objectContaining({ key: "X-Debug", value: "${trace_id}", enabled: true })
        ]),
        jsonpath: expect.stringContaining('"name": "product_id"')
      })
    })));
    expect(await screen.findByText("接口配置已保存。")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled());
    expect(screen.getAllByText("配置已保存").length).toBeGreaterThan(0);
    fireEvent.change(screen.getByPlaceholderText("$.data.token"), { target: { value: "$.data.productId" } });
    fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
    await waitFor(() => expect(apiMock.interfaces.saveConfiguration).toHaveBeenLastCalledWith(1, expect.objectContaining({
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
    await waitFor(() => expect(apiMock.interfaces.saveConfiguration).toHaveBeenLastCalledWith(1, expect.objectContaining({
      revision: 3,
      configuration: expect.objectContaining({
        params: expect.stringContaining('"page": "${page_no}"'),
        paramsMeta: [expect.objectContaining({ key: "page", value: "${page_no}", description: "页码", enabled: true })]
      })
    })));
    await waitFor(() => expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled());
    expect(screen.queryByText("有未保存修改")).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("请求 URL"), { target: { value: "https://example.com/products" } });
    await waitFor(() => {
      expect(screen.getByLabelText("参数名 1")).toHaveValue("");
      expect(screen.getByText("共 0 个参数，已启用 0 个")).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText("参数名 1"), { target: { value: "temporary" } });
    expect(screen.getByText("共 1 个参数，已启用 1 个")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("参数名 1"), { target: { value: "" } });
    expect(screen.getByText("共 0 个参数，已启用 0 个")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^临时变量/ }));
    expect(screen.getByText("保存到当前接口，用于预览、发送和 cURL 导出，优先级最高")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("临时变量名 1"), { target: { value: "session_id" } });
    fireEvent.change(screen.getByLabelText("临时变量值 1"), { target: { value: "debug-session" } });
    fireEvent.change(screen.getByLabelText("临时变量名 2"), { target: { value: "retry_count" } });
    fireEvent.change(screen.getByLabelText("临时变量类型 2"), { target: { value: "number" } });
    fireEvent.change(screen.getByLabelText("临时变量值 2"), { target: { value: "3" } });
    expect(screen.getByText("共 2 个变量，已启用 2 个")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /请求体/ }));
    expect(screen.getByLabelText("请求体数据类型")).toHaveValue("json");
    fireEvent.change(screen.getByLabelText("请求体内容"), { target: { value: "{\"roomNo\":\"391\",\"enabled\":true,\"retry_count\":5}" } });
    fireEvent.click(screen.getByRole("button", { name: "格式化 JSON" }));
    expect(screen.getByLabelText("请求体内容")).toHaveValue("{\n  \"roomNo\": \"391\",\n  \"enabled\": true,\n  \"retry_count\": 5\n}");
    expect(screen.getByText("JSON 格式正确")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "复制到临时变量" }));
    expect(screen.getByText("已复制 3 个字段到临时变量")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^临时变量/ }));
    expect(screen.getByLabelText("临时变量值 2")).toHaveValue("5");
    expect(screen.getByLabelText("临时变量值 3")).toHaveValue("391");
    expect(screen.getByLabelText("临时变量类型 4")).toHaveValue("boolean");
    expect(screen.getByText("共 4 个变量，已启用 4 个")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "保存临时变量" }));
    await waitFor(() => expect(apiMock.interfaces.saveConfiguration).toHaveBeenLastCalledWith(1, expect.objectContaining({
      revision: 4,
      configuration: expect.objectContaining({
        temporaryVariables: expect.arrayContaining([
          expect.objectContaining({ key: "retry_count", type: "number", value: "5", enabled: true }),
          expect.objectContaining({ key: "enabled", type: "boolean", value: "true", enabled: true })
        ])
      })
    })));
    await waitFor(() => expect(screen.getByRole("button", { name: "保存临时变量" })).toBeDisabled());
    fireEvent.click(screen.getByRole("button", { name: /请求体/ }));
    fireEvent.change(screen.getByLabelText("请求体内容"), { target: { value: "{\"roomNo\":" } });
    fireEvent.click(screen.getByRole("button", { name: "格式化 JSON" }));
    expect(screen.getByText("JSON 格式错误，请检查后重试")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "复制到临时变量" }));
    expect(screen.getByText("请求体不是合法 JSON，无法复制")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("请求体内容"), { target: { value: "{\"roomNo\":\"391\"}" } });
    fireEvent.click(screen.getAllByRole("button", { name: /执行/ })[0]);

    await waitFor(() => expect(apiMock.debug.start).toHaveBeenCalled());
    expect(apiMock.debug.start).toHaveBeenCalledWith(1, expect.objectContaining({
      temporaryVariables: { session_id: "debug-session", retry_count: 5, roomNo: "391", enabled: true }
    }));
    expect(await screen.findByText("执行器调试完成。")).toBeInTheDocument();
    expect(apiMock.requestHeaders.list).toHaveBeenCalledWith({ projectId: "8" });
  }, 10000);
});
