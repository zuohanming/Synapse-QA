import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const executionMock = vi.hoisted(() => ({
  list: vi.fn(),
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
    expect(await screen.findByText("执行中")).toBeInTheDocument();
    expect(screen.getByText("0%")).toBeInTheDocument();

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 2100));
    });

    await waitFor(() => expect(document.querySelector(".status-badge")).toHaveTextContent("已通过"));
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(executionMock.list).toHaveBeenCalledTimes(2);
  });
});
