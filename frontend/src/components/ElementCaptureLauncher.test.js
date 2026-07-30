import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ElementCaptureLauncher } from "./ElementCaptureLauncher.js";
import { elementCaptureService } from "../services/elementCaptureService.js";

vi.mock("../services/elementCaptureService.js", () => ({
  elementCaptureService: {
    create: vi.fn()
  }
}));

const pageRow = {
  id: 8,
  name: "登录页",
  locator: "https://example.test/login?access_token=secret#account"
};

const executors = [
  { executorId: "offline", name: "离线执行器", status: "offline", supportedTypes: ["ui"] },
  { executorId: "api", name: "接口执行器", status: "online", supportedTypes: ["api"] },
  { executorId: "ui-1", name: "北京 UI 执行器", status: "online", supportedTypes: ["ui"] },
  { executorId: "browser-1", name: "浏览器执行器", status: "online", supportedTypes: ["browser"] }
];

describe("ElementCaptureLauncher", () => {
  beforeEach(() => {
    elementCaptureService.create.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("只呈现在线且声明 UI 能力的执行器并隐藏 URL 敏感部分", () => {
    render(<ElementCaptureLauncher pageRow={pageRow} executors={executors} />);

    expect(screen.getByRole("dialog", { name: "启动页面元素采集" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /北京 UI 执行器/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /浏览器执行器/ })).not.toBeInTheDocument();
    expect(screen.queryByText("离线执行器")).not.toBeInTheDocument();
    expect(screen.queryByText("接口执行器")).not.toBeInTheDocument();
    expect(screen.getByText("https://example.test/login")).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("access_token");
    expect(document.body.textContent).not.toContain("secret");
  });

  it("无可用执行器时提供配置引导且不能启动", () => {
    render(<ElementCaptureLauncher pageRow={pageRow} executors={executors.slice(0, 2)} />);

    expect(screen.getByText("没有可用于有头采集的在线执行器。")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "前往执行器配置" })).toHaveAttribute("href", "/config");
    expect(screen.getByRole("button", { name: "启动有头采集" })).toBeDisabled();
  });

  it("启动期间阻止重复提交并在成功后只传递非敏感 session", async () => {
    let resolveCreate;
    elementCaptureService.create.mockImplementation(() => new Promise((resolve) => {
      resolveCreate = resolve;
    }));
    const onStarted = vi.fn();
    render(<ElementCaptureLauncher pageRow={pageRow} executors={executors} onStarted={onStarted} />);

    const submit = screen.getByRole("button", { name: "启动有头采集" });
    fireEvent.click(submit);
    fireEvent.click(submit);
    expect(elementCaptureService.create).toHaveBeenCalledTimes(1);
    expect(submit).toBeDisabled();
    expect(elementCaptureService.create).toHaveBeenCalledWith(
      expect.objectContaining({
        pageId: 8,
        executorId: "ui-1",
        browserChannel: "chrome",
        mode: "pick",
        url: "https://example.test/login"
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );

    resolveCreate({ id: "session-1", status: "starting", token: "must-not-forward", receipt: "must-not-forward" });
    await waitFor(() => expect(onStarted).toHaveBeenCalledTimes(1));
    const forwarded = onStarted.mock.calls[0][0];
    expect(forwarded).toMatchObject({
      id: "session-1",
      pageName: "登录页",
      executorName: "北京 UI 执行器"
    });
    expect(forwarded).not.toHaveProperty("token");
    expect(forwarded).not.toHaveProperty("receipt");
  });

  it("启动失败保留选择并显示错误", async () => {
    elementCaptureService.create.mockRejectedValue(new Error("执行器正在采集页面元素"));
    render(<ElementCaptureLauncher pageRow={pageRow} executors={executors} />);

    fireEvent.click(screen.getByRole("button", { name: "Microsoft Edge" }));
    fireEvent.click(screen.getByRole("button", { name: "启动有头采集" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("执行器正在采集页面元素");
    expect(screen.getByRole("button", { name: /北京 UI 执行器/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Microsoft Edge" })).toHaveAttribute("aria-pressed", "true");
  });

  it("Escape 关闭并把焦点归还给启动入口", () => {
    const opener = document.createElement("button");
    opener.textContent = "自动采集";
    document.body.appendChild(opener);
    opener.focus();
    const onClose = vi.fn();
    render(<ElementCaptureLauncher pageRow={pageRow} executors={executors} onClose={onClose} />);

    expect(screen.getByRole("button", { name: "关闭启动弹窗" })).toHaveFocus();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(opener).toHaveFocus();
    opener.remove();
  });
});
