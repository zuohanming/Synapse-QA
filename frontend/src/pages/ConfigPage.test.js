import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const configMock = vi.hoisted(() => ({
  executorToken: { get: vi.fn(), generate: vi.fn() }
}));
const executionMock = vi.hoisted(() => ({ executors: vi.fn(), createExecutor: vi.fn(), generateExecutorToken: vi.fn() }));

vi.mock("../services/configService.js", () => ({ configService: configMock }));
vi.mock("../services/executionService.js", () => ({ executionService: executionMock }));

import { ConfigPage } from "./ConfigPage.js";

describe("执行器配置页", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("展示执行器状态和负载并支持刷新", async () => {
    configMock.executorToken.get.mockResolvedValue({ maskedToken: "syn***token", updatedAt: "2026-07-23T09:00:00+08:00" });
    executionMock.executors.mockResolvedValue([{
      executorId: "local-python-executor",
      name: "本地 Python 执行器",
      endpoint: "http://127.0.0.1:8090",
      status: "online",
      maxWorkers: 2,
      runningTasks: 1,
      queuedTasks: 3,
      supportedTypes: ["ui", "api"],
      lastHeartbeatAt: "2026-07-23T09:01:00+08:00"
    }]);
    executionMock.generateExecutorToken.mockResolvedValue({ executorId: "local-python-executor", token: "executor_unique_token" });

    render(<ConfigPage activePath={["配置管理", "执行器配置"]} />);

    expect(await screen.findByText("本地 Python 执行器")).toBeInTheDocument();
    expect(screen.getByText("在线")).toBeInTheDocument();
    expect(screen.getByText("运行中").parentElement).toHaveTextContent("运行中1");
    expect(screen.getByText("排队中").parentElement).toHaveTextContent("排队中3");
    expect(screen.getByText("在线 1 / 共 1 个，状态每 10 秒自动更新")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "刷新状态" }));
    await waitFor(() => expect(executionMock.executors).toHaveBeenCalledTimes(2));

    fireEvent.click(screen.getByRole("button", { name: "生成 Token" }));
    expect(await screen.findByText("executor_unique_token")).toBeInTheDocument();
    expect(executionMock.generateExecutorToken).toHaveBeenCalledWith("local-python-executor");
  });

  it("没有执行器时给出启动指引", async () => {
    configMock.executorToken.get.mockResolvedValue({ maskedToken: "未生成" });
    executionMock.executors.mockResolvedValue([]);

    render(<ConfigPage activePath={["配置管理", "执行器配置"]} />);

    expect(await screen.findByText("暂无已注册执行器，请启动执行器并确认 Token 配置正确。")).toBeInTheDocument();
  });

  it("新增执行器并生成专属 Token", async () => {
    configMock.executorToken.get.mockResolvedValue({});
    executionMock.executors.mockResolvedValueOnce([]).mockResolvedValue([{
      executorId: "executor-beijing-01",
      name: "北京 UI 执行器",
      status: "pending",
      supportedTypes: []
    }]);
    executionMock.createExecutor.mockResolvedValue({ executorId: "executor-beijing-01", token: "executor_new_token" });
    render(<ConfigPage activePath={["配置管理", "执行器配置"]} />);
    await screen.findByText("暂无已注册执行器，请启动执行器并确认 Token 配置正确。");

    fireEvent.click(screen.getByRole("button", { name: "新增执行器" }));
    fireEvent.change(screen.getByPlaceholderText("例如 executor-beijing-01"), { target: { value: "executor-beijing-01" } });
    fireEvent.change(screen.getByPlaceholderText("例如 北京 UI 执行器"), { target: { value: "北京 UI 执行器" } });
    fireEvent.click(screen.getByRole("button", { name: "创建并生成 Token" }));

    expect(await screen.findByText("executor_new_token")).toBeInTheDocument();
    expect(executionMock.createExecutor).toHaveBeenCalledWith({ executorId: "executor-beijing-01", name: "北京 UI 执行器" });
  });
});
