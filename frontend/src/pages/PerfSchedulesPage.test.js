import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { serializeScheduleCreate, serializeSchedulePatch } from "./PerfSchedulesPage.js";
import { PerfSchedulesPage } from "./PerfSchedulesPage.js";
import { menuIcons } from "../components/Layout.js";

vi.mock("../hooks/useAuth.js", () => ({ useAuth: () => ({ user: { permissions: ["perf.plan.manage", "perf.plan.execute"] } }) }));
vi.mock("../hooks/useAsyncData.js", () => ({ useAsyncData: () => ({ data: [], loading: false, error: "", reload: vi.fn() }) }));
vi.mock("../services/performanceService.js", () => ({ performanceService: { schedules: { list: vi.fn() }, plans: { list: vi.fn() } } }));

describe("定时规则 payload", () => {
  it("create/edit 使用 numeric plan/environment ID 并携带 enabled", () => {
    expect(serializeScheduleCreate({ name: "nightly", planId: "12", environmentId: "8", cronExpression: "0 1 * * *", timezone: "Asia/Shanghai", enabled: true })).toEqual({ name: "nightly", planId: 12, environmentId: 8, cronExpression: "0 1 * * *", timezone: "Asia/Shanghai", enabled: true });
    const patch = serializeSchedulePatch({ name: "follow", planId: "12", environmentId: "", cronExpression: "0 1 * * *", timezone: "Asia/Shanghai", enabled: false });
    expect(patch.environmentId).toBe(null);
    expect(patch).not.toHaveProperty("enabled");
  });

  it("复用单面板结构，空态显示分页且菜单有定时图标", () => {
    const { container } = render(<PerfSchedulesPage />);
    expect(container.querySelector(".page-header")).not.toBeInTheDocument();
    expect(container.querySelector(".perf-schedule-eyebrow")).not.toBeInTheDocument();
    expect(container.querySelector("h1")).not.toBeInTheDocument();
    expect(screen.getByText("定时规则管理")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "搜索" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重置" })).toBeInTheDocument();
    expect(container.querySelector(".perf-schedule-summary")).not.toBeInTheDocument();
    expect(screen.getByText("暂无定时规则")).toBeInTheDocument();
    expect(container.querySelectorAll(".section-stack > .resource-panel")).toHaveLength(1);
    expect(container.querySelector(".section-stack > .resource-panel .table-panel")).toBeInTheDocument();
    expect(container.querySelector(".perf-schedule-table-panel")).not.toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "分页导航" })).toBeInTheDocument();
    expect(screen.getByText("共 0 条")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "第 1 页" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "上一页" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "下一页" })).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "每页条数" })).toBeInTheDocument();
    expect(menuIcons["定时规则"]).toBeTruthy();
  });
});
