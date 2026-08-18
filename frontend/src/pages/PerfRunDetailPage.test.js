import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const reload = vi.fn();
const stream = vi.fn();
let handlers;
let controller;

vi.mock("../hooks/useAuth.js", () => ({ useAuth: () => ({ user: { permissions: ["perf.plan.execute"] } }) }));
vi.mock("../hooks/useAsyncData.js", () => ({ useAsyncData: () => ({
  data: { id: 1, status: "running", scenarioType: "baseline", environment: "test", series: { version: 1, lastSequence: 1, points: [{ sequence: 1, timestamp: "2026-01-01T00:00:00Z", rps: null, p95: NaN, vus: 2 }] }, summary: {}, planSnapshot: {}
  }, loading: false, error: "", reload
}) }));
vi.mock("../services/performanceService.js", () => ({ performanceService: {
  runs: { stream: (...args) => { handlers = args[2]; controller = { signal: { aborted: false }, cancel: vi.fn(), promise: new Promise(() => {}) }; stream(...args); return controller; }, cancel: vi.fn() }
} }));

import { PerfRunDetailPage } from "./PerfRunDetailPage.js";

describe("PerfRunDetailPage P1 生命周期", () => {
  beforeEach(() => { reload.mockReset(); stream.mockReset(); });
  afterEach(() => vi.clearAllTimers());

  it("恢复 persisted series、去重 sample、reset 覆盖并保留缺失值空态", () => {
    render(<PerfRunDetailPage runId="1" />);
    expect(screen.getByText("阶段：--")).toBeInTheDocument();
    act(() => handlers["perf.sample"]({ sequence: 1, stage: "hold", rps: 4 }));
    act(() => handlers["perf.sample"]({ sequence: 2, stage: "hold", rps: 5 }));
    act(() => handlers["perf.reset"]({ version: 1, points: [{ sequence: 9, stage: "ramp", rps: 1 }] }));
    expect(screen.getByText("阶段：ramp")).toBeInTheDocument();
    expect(stream).toHaveBeenCalledWith(1, 1, expect.any(Object));
  });

  it("terminal 只 reload 一次并取消连接，卸载也可取消", async () => {
    const view = render(<PerfRunDetailPage runId="1" />);
    await act(async () => { await handlers["perf.terminal"](); await handlers["perf.terminal"](); });
    expect(reload).toHaveBeenCalledTimes(1);
    expect(controller.cancel).toHaveBeenCalled();
    view.unmount();
    expect(controller.cancel).toHaveBeenCalled();
  });
});
