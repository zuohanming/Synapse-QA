import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { overview, user } = vi.hoisted(() => ({
  overview: vi.fn(),
  user: { roleCode: "member", permissions: ["menu.execution.read", "menu.api_automation.read"] }
}));

vi.mock("../services/dashboardService.js", () => ({ dashboardService: { overview } }));
vi.mock("../hooks/useAuth.js", () => ({ useAuth: () => ({ user }) }));

import { DashboardPage, getDashboardItemKey, getPartialMessage, hasRunningExecutions, navigateToTarget } from "./DashboardPage.js";
import { pathToHash } from "../utils/routeState.js";

const response = {
  generatedAt: "2026-08-19T10:00:00.000Z",
  projects: [{ id: 7, name: "商城项目" }],
  executions: {
    state: "ok",
    sourceStates: { ui: "ok", api: "ok", perf: "ok" },
    counts: { total: 3, success: 2, failed: 1, running: 0, canceled: 0, unknown: 0 },
    recent: [
      { id: 1, type: "api", title: "登录接口回归", projectName: "商城项目", status: "success", startedAt: "2026-08-19T09:55:00.000Z", durationMs: 42000 },
      { id: 2, type: "ui", title: "首页关键流程", projectName: "商城项目", status: "failed", startedAt: "2026-08-19T09:40:00.000Z", durationMs: 72000 },
      { id: 3, type: "perf", title: "订单峰值压测", projectName: "商城项目", status: "running", startedAt: "2026-08-19T09:00:00.000Z" }
    ]
  },
  attention: { state: "ok", total: 1, items: [{ id: "a1", type: "failed", title: "首页关键流程失败", description: "商城项目 · 18 分钟前", projectName: "商城项目" }] }
};

beforeEach(() => {
  overview.mockReset();
  overview.mockResolvedValue(response);
  user.permissions = ["menu.execution.read", "menu.api_automation.read"];
  window.location.hash = "";
});
afterEach(cleanup);

describe("DashboardPage", () => {
  it("渲染指标、待办和最近执行", async () => {
    render(<DashboardPage />);
    expect(await screen.findByText("项目概览")).toBeInTheDocument();
    expect(screen.getByText("66.7%")).toBeInTheDocument();
    expect(screen.getByText("失败执行")).toBeInTheDocument();
    expect(screen.getByText("首页关键流程失败")).toBeInTheDocument();
    expect(screen.getAllByText("登录接口回归").length).toBeGreaterThan(0);
    expect(screen.getAllByText("订单峰值压测").length).toBeGreaterThan(0);
    expect(within(screen.getByRole("region", { name: "最近执行" })).getByRole("button", { name: /查看全部/ })).toBeInTheDocument();
  });

  it("传递项目和时间筛选参数", async () => {
    render(<DashboardPage />);
    await screen.findAllByText("登录接口回归");
    fireEvent.change(screen.getAllByLabelText("项目筛选")[0], { target: { value: "7" } });
    fireEvent.change(screen.getAllByLabelText("时间范围")[0], { target: { value: "30d" } });
    await waitFor(() => expect(overview).toHaveBeenLastCalledWith({ projectId: "7", range: "30d" }));
  });

  it("total 为 0 时成功率显示破折号，并支持 partial 提醒", async () => {
    overview.mockResolvedValueOnce({ ...response, executions: { ...response.executions, state: "partial", counts: { total: 0, success: 0, failed: 0, running: 0 }, recent: [] }, attention: { items: [] } });
    render(<DashboardPage />);
    expect((await screen.findAllByText("—")).length).toBeGreaterThan(0);
    expect(screen.getByText(/部分执行数据暂时无法加载/)).toBeInTheDocument();
    expect(screen.getAllByText("暂无执行记录").length).toBeGreaterThan(0);
  });

  it("手动刷新期间保留已有内容", async () => {
    overview.mockReset().mockResolvedValueOnce(response).mockImplementationOnce(() => new Promise(() => {}));
    render(<DashboardPage />);
    await screen.findAllByText("登录接口回归");
    fireEvent.click(screen.getByRole("button", { name: "刷新" }));
    expect(screen.getAllByText("登录接口回归").length).toBeGreaterThan(0);
  });

  it("普通用户隐藏无权限快捷入口，移动端结构保留摘要卡片", async () => {
    render(<DashboardPage />);
    await screen.findAllByText("登录接口回归");
    expect(screen.queryByText("创建压测方案")).not.toBeInTheDocument();
    expect(screen.getAllByText("查看执行记录").length).toBeGreaterThan(0);
    expect(document.querySelector(".dashboard-execution-cards")).toBeInTheDocument();
  });

  it("无执行记录权限时指标保持视觉但不可点击", async () => {
    user.permissions = ["menu.api_automation.read"];
    overview.mockResolvedValue({ ...response, executions: { ...response.executions, recent: [{ ...response.executions.recent[0], targetUrl: "#/接口自动化/接口管理" }] } });
    render(<DashboardPage />);
    await screen.findAllByText("登录接口回归");
    expect(document.querySelectorAll(".dashboard-metric-card").length).toBe(3);
    expect(document.querySelectorAll(".dashboard-metric-card button").length).toBe(0);
    expect(Array.from(document.querySelectorAll(".dashboard-metric-card")).every((node) => node.tagName === "DIV")).toBe(true);
    const recent = screen.getByRole("region", { name: "最近执行" });
    expect(within(recent).queryByRole("button", { name: /查看全部/ })).not.toBeInTheDocument();
    fireEvent.click(within(recent).getAllByRole("button", { name: "详情" })[0]);
    expect(window.location.hash).toBe(pathToHash(["接口自动化", "接口管理"]));
  });
});

describe("Dashboard navigation", () => {
  it("只接受合法站内 hash，外部 targetUrl 回退到执行记录", () => {
    navigateToTarget("https://evil.example/redirect", ["执行中心", "执行记录"]);
    expect(window.location.hash).toBe(pathToHash(["执行中心", "执行记录"]));
    navigateToTarget("#/performance/plans/new", ["执行中心", "执行记录"]);
    expect(window.location.hash).toBe("#/performance/plans/new");
  });

  it("识别来源错误为局部提醒", () => {
    expect(getPartialMessage({ executions: { sourceStates: { api: { state: "error" } } } })).toContain("部分执行数据");
    expect(getPartialMessage({ executions: { state: "partial", sourceStates: { ui: "forbidden", perf: "forbidden" } } })).toBe("");
    expect(getPartialMessage({ executions: { state: "ok", sourceStates: { ui: "forbidden", perf: "forbidden" } } })).toBe("");
  });

  it("使用 uid 或带来源的安全回退 key", () => {
    expect(getDashboardItemKey({ uid: "run-1", type: "api", id: 8 })).toBe("run-1");
    expect(getDashboardItemKey({ type: "ui", id: 8 })).not.toBe(getDashboardItemKey({ type: "api", id: 8 }));
    expect(getDashboardItemKey({ source: "perf", id: 8 })).toBe("perf:8");
    expect(getDashboardItemKey({ type: "api", source: "api", id: 8 })).toBe("api:8");
  });

  it("按 counts.running 判断是否需要自动刷新", () => {
    expect(hasRunningExecutions({ executions: { counts: { running: 1 }, recent: [] } })).toBe(true);
    expect(hasRunningExecutions({ executions: { counts: { running: 0 }, recent: [{ status: "running" }] } })).toBe(false);
  });

  it("counts.running 大于 0 时即使 recent 前五条没有 running 也会注册刷新", async () => {
    vi.useFakeTimers();
    try {
      const noRunningRecent = { ...response, executions: { ...response.executions, counts: { ...response.executions.counts, running: 1 }, recent: response.executions.recent.map((item) => ({ ...item, status: "success" })) } };
      overview.mockResolvedValue(noRunningRecent);
      render(<DashboardPage />);
      await act(async () => { await Promise.resolve(); await Promise.resolve(); });
      expect(overview).toHaveBeenCalledTimes(1);
      await act(async () => { vi.advanceTimersByTime(30000); await Promise.resolve(); });
      expect(overview).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
