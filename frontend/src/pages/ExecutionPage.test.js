import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const executionMock = vi.hoisted(() => ({
  list: vi.fn(),
  statistics: vi.fn(),
  get: vi.fn(),
  cancel: vi.fn()
}));

vi.mock("../services/executionService.js", () => ({ executionService: executionMock }));

import { ExecutionPage } from "./ExecutionPage.js";

describe("执行记录自动刷新", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("执行中批次完成后自动更新状态", async () => {
    executionMock.list
      .mockResolvedValueOnce({ items: [{ id: 7, runType: "ui", status: "running", summary: { total: 1 } }], total: 1 })
      .mockResolvedValue({ items: [{ id: 7, runType: "ui", status: "completed", summary: { total: 1, passed: 1, failed: 0 } }], total: 1 });

    render(<ExecutionPage activePath={["执行中心", "执行记录"]} />);
    await waitFor(() => expect(document.querySelector(".status-badge")).toHaveTextContent("执行中"));
    expect(screen.getByText("0%")).toBeInTheDocument();

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 2100));
    });

    await waitFor(() => expect(document.querySelector(".status-badge")).toHaveTextContent("已通过"));
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(executionMock.list).toHaveBeenCalledTimes(2);
  });

  it("展示丰富的测试结论、失败定位与日志明细", async () => {
    executionMock.statistics.mockResolvedValue({
      totalRuns: 28,
      totalCases: 460,
      passedCases: 430,
      failedCases: 30,
      failedRuns: 3,
      runningRuns: 1,
      passRate: 93.5,
      runChange: 12.5,
      caseChange: -4.2,
      passRateChange: 2.1,
      trend: Array.from({ length: 14 }, (_, index) => ({ date: `07-${String(index + 10).padStart(2, "0")}`, runs: 2, cases: 20 + index, passRate: 90 }))
    });
    executionMock.list.mockResolvedValue({
      items: [{ id: 18, runType: "ui", status: "failed", summary: { total: 2, passed: 1, failed: 1, skipped: 0 } }],
      total: 1
    });
    executionMock.get.mockResolvedValue({
      id: 18,
      runType: "ui",
      status: "failed",
      triggeredBy: "tester",
      startedAt: "2026-07-23T08:00:00.000Z",
      finishedAt: "2026-07-23T08:00:12.000Z",
      summary: { total: 2, passed: 1, failed: 1, skipped: 0 },
      tasks: [
        { id: 1, caseId: 101, executorId: 3, status: "success", startedAt: "2026-07-23T08:00:00.000Z", finishedAt: "2026-07-23T08:00:05.000Z", result: { output: "登录成功" } },
        { id: 2, caseId: 102, executorId: 4, status: "failed", startedAt: "2026-07-23T08:00:05.000Z", finishedAt: "2026-07-23T08:00:12.000Z", result: { output: "开始校验", error: "未找到提交按钮" } }
      ]
    });

    render(<ExecutionPage activePath={["执行中心", "测试报告"]} />);
    expect(await screen.findByText("93.5")).toBeInTheDocument();
    expect(screen.getByText("累计执行批次")).toBeInTheDocument();
    expect(screen.getByText("近 14 天质量脉冲")).toBeInTheDocument();
    const reportButton = await screen.findByRole("button", { name: "查看报告" });
    reportButton.click();

    expect(await screen.findByText("50%")).toBeInTheDocument();
    expect(screen.getByText("存在失败用例，需要关注")).toBeInTheDocument();
    expect(screen.getByText("失败定位")).toBeInTheDocument();
    expect(screen.getAllByText("未找到提交按钮").length).toBeGreaterThan(0);
    expect(screen.getByText("开始校验")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "导出 HTML" })).toBeInTheDocument();
  });
});
