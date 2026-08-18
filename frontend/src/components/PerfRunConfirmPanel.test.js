import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const { runMock, hookState } = vi.hoisted(() => ({ runMock: vi.fn().mockResolvedValue({ id: 9 }), hookState: { count: 0 } }));
vi.mock("../hooks/useAsyncData.js", () => ({ useAsyncData: () => { hookState.count += 1; return { data: hookState.count === 2 ? [{ environmentId: 7, envName: "预发", deployEnv: "staging", baseUrl: "https://staging.example.com" }] : [], loading: false, error: "", reload: vi.fn() }; } }));
vi.mock("../services/performanceService.js", () => ({ performanceService: { executors: vi.fn(), environments: vi.fn(), runs: { list: vi.fn() }, plans: { run: runMock } } }));

import { PerfRunConfirmPanel } from "./PerfRunConfirmPanel.js";

describe("执行确认执行器文案", () => {
  it("不显示误导性的执行器选择控件", () => {
    render(<PerfRunConfirmPanel plan={{ id: 1, name: "方案", scenarioType: "baseline", environment: "test", loadConfig: { vus: 10, duration: "1m" }, thresholds: [] }} onClose={vi.fn()} onRun={vi.fn()} />);
    expect(screen.getByText("平台自动调度执行器")).toBeInTheDocument();
    expect(screen.queryByText("请选择执行器")).not.toBeInTheDocument();
    fireEvent.change(screen.getAllByRole("combobox")[0], { target: { value: "7" } });
    fireEvent.click(screen.getByRole("button", { name: /确认执行/ }));
    expect(runMock).toHaveBeenCalledWith(1, expect.any(String), { environmentId: 7 });
  });
});
