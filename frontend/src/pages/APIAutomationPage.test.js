import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({
  interfaces: { list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn() },
  requestHeaders: { list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn() }
}));
const configMock = vi.hoisted(() => ({
  projects: { list: vi.fn() },
  products: { list: vi.fn() },
  productModules: { list: vi.fn() }
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
    const fetchMock = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      headers: new Headers(),
      text: vi.fn().mockResolvedValue("{}")
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<APIAutomationPage activePath={["接口自动化", "接口管理"]} />);
    await screen.findByText("查询商品");
    fireEvent.click(screen.getByRole("button", { name: "调试" }));
    expect(await screen.findByText("已加载 2 个项目默认请求头；接口内同名请求头优先。")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: /执行/ })[0]);

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(fetchMock.mock.calls[0][1].headers).toEqual({
      Authorization: "Bearer interface",
      "X-Project": "mall"
    });
    expect(apiMock.requestHeaders.list).toHaveBeenCalledWith({ projectId: "8" });
  });
});
