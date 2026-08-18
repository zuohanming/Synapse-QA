import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn().mockResolvedValue({ id: 1, name: "订单", method: "GET", path: "/orders", configuration: {} }) }));
vi.mock("../hooks/useAsyncData.js", () => ({ useAsyncData: () => ({ data: { items: [{ id: 1, name: "订单", method: "GET", path: "/orders" }, { id: 2, name: "流式", method: "HEAD", path: "/head" }, { id: 3, name: "用户", method: "GET", path: "/users" }], total: 3 }, loading: false, error: "", reload: vi.fn() }) }));
vi.mock("../services/apiAutomationService.js", () => ({ apiAutomationService: { interfaces: { list: vi.fn(), get: getMock } } }));

import { PerfInterfacePicker } from "./PerfInterfacePicker.js";

describe("性能接口选择器", () => {
  it("展示搜索、选中数量并禁止选择不支持方法", async () => {
    const onConfirm = vi.fn();
    render(<PerfInterfacePicker maxSelection={1} productId="1" multiple onClose={vi.fn()} onConfirm={onConfirm} />);
    expect(screen.getByText("导入后为独立快照，后续编辑不会联动接口管理。")).toBeInTheDocument();
    expect(screen.getByLabelText("选择 流式")).toBeDisabled();
    fireEvent.click(screen.getByLabelText("选择 订单"));
    expect(screen.getByText(/已选 1 个/)).toBeInTheDocument();
    expect(screen.getByLabelText("选择 用户")).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "导入已选" }));
    expect(getMock).toHaveBeenCalledWith(1);
  });
});
