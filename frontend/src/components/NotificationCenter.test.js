import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const notificationMock = vi.hoisted(() => ({
  list: vi.fn(), unreadCount: vi.fn(), markRead: vi.fn(), markAllRead: vi.fn(), remove: vi.fn(), preferences: vi.fn(), updatePreferences: vi.fn()
}));
vi.mock("../services/notificationService.js", () => ({ notificationService: notificationMock }));
import { NotificationCenter } from "./NotificationCenter.js";

describe("通知中心", () => {
  afterEach(() => { cleanup(); vi.clearAllMocks(); vi.unstubAllGlobals(); });

  it("展示未读角标并支持读取通知", async () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise(() => {})));
    notificationMock.list.mockResolvedValue({ items: [{ id: 1, title: "测试执行失败", content: "批次 #9 执行失败", type: "execution.failure", level: "error", isRead: false, targetUrl: "#/执行中心/执行记录", createdAt: new Date().toISOString() }] });
    notificationMock.unreadCount.mockResolvedValue({ count: 1 });
    notificationMock.markRead.mockResolvedValue({});

    render(<NotificationCenter />);
    expect(await screen.findByText("1")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "通知" }));
    fireEvent.click(await screen.findByText("测试执行失败"));

    await waitFor(() => expect(notificationMock.markRead).toHaveBeenCalledWith(1));
    expect(decodeURIComponent(window.location.hash)).toContain("执行中心");
  });
});
