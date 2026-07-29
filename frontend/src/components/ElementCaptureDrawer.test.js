import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  ElementCaptureDrawer,
  getPollDelay,
  getSaveBlockers,
  mergeCaptureCandidates
} from "./ElementCaptureDrawer.js";
import { elementCaptureService } from "../services/elementCaptureService.js";

vi.mock("../services/elementCaptureService.js", () => ({
  elementCaptureService: {
    get: vi.fn(),
    candidates: vi.fn(),
    update: vi.fn(),
    save: vi.fn(),
    mode: vi.fn(),
    stop: vi.fn()
  }
}));

const session = {
  id: "session-1",
  pageId: 8,
  pageName: "登录页",
  executorId: "exec-1",
  executorName: "北京 UI 执行器",
  status: "active",
  mode: "pick",
  candidateCount: 4,
  currentUrl: "https://example.test/login?access_token=session-secret",
  token: "session-token",
  receipt: "command-receipt"
};

function candidate(cursorId, overrides = {}) {
  return {
    id: `private-${cursorId}`,
    cursorId,
    sessionId: "session-1",
    name: `元素 ${cursorId}`,
    fingerprint: `fingerprint-secret-${cursorId}`,
    captureUrl: `https://example.test/private?token=${cursorId}`,
    tagName: "button",
    accessibleName: `元素 ${cursorId}`,
    locators: [{ type: "testid", value: `item-${cursorId}`, score: 95, unique: true }],
    qualityScore: 95,
    duplicateElementId: 0,
    conflictStatus: "",
    conflictResolution: "",
    status: "pending",
    ...overrides
  };
}

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe("ElementCaptureDrawer", () => {
  beforeEach(() => {
    Object.values(elementCaptureService).forEach((mock) => mock.mockReset());
    elementCaptureService.get.mockResolvedValue(session);
    elementCaptureService.candidates.mockResolvedValue([]);
    elementCaptureService.update.mockResolvedValue({ message: "已更新" });
    elementCaptureService.mode.mockResolvedValue({ message: "已更新" });
    elementCaptureService.stop.mockResolvedValue({ message: "已停止" });
    elementCaptureService.save.mockResolvedValue({ savedCandidateIds: [], ignoredCandidateIds: [] });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("按 cursor 去重升序合并并采用有上限的断网退避", () => {
    const merged = mergeCaptureCandidates(
      [candidate(3, { name: "旧名称" }), candidate(1)],
      [candidate(3, { name: "新名称" }), candidate(2)]
    );

    expect(merged.map((item) => item.cursorId)).toEqual([1, 2, 3]);
    expect(merged[2].name).toBe("新名称");
    expect([0, 1, 2, 3, 4, 5].map(getPollDelay)).toEqual([2000, 4000, 8000, 16000, 30000, 30000]);
  });

  it("切换 session 后丢弃旧响应", async () => {
    const oldResponse = deferred();
    elementCaptureService.candidates.mockImplementation((sessionId) => (
      sessionId === "session-old" ? oldResponse.promise : Promise.resolve([candidate(2, { name: "新会话元素" })])
    ));
    elementCaptureService.get.mockImplementation((sessionId) => Promise.resolve({
      ...session,
      id: sessionId
    }));
    const { rerender } = render(
      <ElementCaptureDrawer session={{ ...session, id: "session-old" }} />
    );

    rerender(<ElementCaptureDrawer session={{ ...session, id: "session-new" }} />);
    expect(await screen.findByDisplayValue("新会话元素")).toBeInTheDocument();
    await act(async () => oldResponse.resolve([candidate(1, { name: "旧会话元素" })]));

    expect(screen.queryByDisplayValue("旧会话元素")).not.toBeInTheDocument();
  });

  it("终态停止轮询，断网按 2/4 秒节奏退避且保留已有候选", async () => {
    vi.useFakeTimers();
    elementCaptureService.get
      .mockResolvedValueOnce(session)
      .mockResolvedValueOnce(session)
      .mockResolvedValueOnce({ ...session, status: "completed" });
    elementCaptureService.candidates
      .mockResolvedValueOnce([candidate(1, { name: "已接收元素" })])
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce([]);
    render(<ElementCaptureDrawer session={session} />);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByDisplayValue("已接收元素")).toBeInTheDocument();
    expect(elementCaptureService.candidates).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(2000);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(elementCaptureService.candidates).toHaveBeenCalledTimes(2);
    expect(screen.getByDisplayValue("已接收元素")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("已有候选仍可审核");

    await act(async () => {
      vi.advanceTimersByTime(3999);
      await Promise.resolve();
    });
    expect(elementCaptureService.candidates).toHaveBeenCalledTimes(2);

    await act(async () => {
      vi.advanceTimersByTime(1);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(elementCaptureService.candidates).toHaveBeenCalledTimes(3);
    expect(screen.getByText("会话已停止，候选仍需保存或忽略。")).toBeInTheDocument();

    await act(async () => {
      vi.advanceTimersByTime(30000);
      await Promise.resolve();
    });
    expect(elementCaptureService.candidates).toHaveBeenCalledTimes(3);
  });

  it("质量轨道计算数量并过滤候选，同时展示 400/500 容量提示", async () => {
    elementCaptureService.get.mockResolvedValue({ ...session, candidateCount: 500 });
    elementCaptureService.candidates.mockResolvedValue([
      candidate(1, { name: "可保存" }),
      candidate(2, { name: "未命名元素" }),
      candidate(3, { name: "弱定位", locators: [{ type: "css", value: ".item", score: 65, unique: false }] }),
      candidate(4, { name: "重复元素", conflictStatus: "duplicate", duplicateElementId: 42 })
    ]);
    render(<ElementCaptureDrawer session={session} />);

    expect(await screen.findByRole("button", { name: "可保存 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "待命名 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "定位不可靠 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "冲突 1" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "待命名 1" }));
    expect(screen.getByDisplayValue("未命名元素")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("可保存")).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("500");
    expect(screen.getByRole("alert")).toHaveTextContent("暂停拾取");
  });

  it("候选达到 400 个时显示接近上限的非阻塞预警", async () => {
    elementCaptureService.get.mockResolvedValue({ ...session, candidateCount: 400 });
    render(<ElementCaptureDrawer session={session} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("已达到 400 个");
    expect(screen.getByRole("alert")).toHaveTextContent("接近 500 个上限");
  });

  it("保存门禁覆盖未命名、可靠唯一定位、冲突 target、200 上限和 400 预警", () => {
    expect(getSaveBlockers([candidate(1, { name: "未命名元素" })], new Set([1]))).toContain("1 个候选尚未命名");
    expect(getSaveBlockers([
      candidate(1, { locators: [{ type: "css", value: ".item", score: 95, unique: false }] })
    ], new Set([1]))).toContain("1 个候选缺少评分不低于 70 的唯一定位器");
    expect(getSaveBlockers([
      candidate(1, { conflictStatus: "duplicate", duplicateElementId: 42 })
    ], new Set([1]))).toContain("1 个候选尚未处理冲突");
    expect(getSaveBlockers([
      candidate(1, {
        conflictStatus: "duplicate",
        conflictResolution: "update",
        conflictTargetIds: [42, 43],
        targetElementId: 0
      })
    ], new Set([1]))).toContain("1 个更新候选需要选择目标元素");
    const twoHundredOne = Array.from({ length: 201 }, (_, index) => candidate(index + 1));
    expect(getSaveBlockers(twoHundredOne, new Set(twoHundredOne.map((item) => item.cursorId)))).toContain("一次最多保存 200 个候选");
  });

  it("冲突支持 update/ignore/create，多目标 update 必须选择 target", async () => {
    elementCaptureService.candidates.mockResolvedValue([
      candidate(4, {
        name: "重复元素",
        conflictStatus: "duplicate",
        duplicateElementId: 42,
        conflictTargetIds: [42, 43]
      })
    ]);
    render(<ElementCaptureDrawer session={session} />);
    const row = await screen.findByTestId("element-capture-candidate-4");

    fireEvent.click(within(row).getByRole("checkbox"));
    fireEvent.click(within(row).getByRole("button", { name: "更新已有元素" }));
    expect(within(row).getByRole("button", { name: "更新已有元素" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("1 个更新候选需要选择目标元素")).toBeInTheDocument();
    fireEvent.change(within(row).getByLabelText("更新目标"), { target: { value: "43" } });
    expect(screen.queryByText("1 个更新候选需要选择目标元素")).not.toBeInTheDocument();
    fireEvent.click(within(row).getByRole("button", { name: "忽略候选" }));
    expect(within(row).getByRole("button", { name: "忽略候选" })).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(within(row).getByRole("button", { name: "另存为新元素" }));
    expect(within(row).getByRole("button", { name: "另存为新元素" })).toHaveAttribute("aria-pressed", "true");
  });

  it("名称内联更新失败时回滚并保留可操作错误", async () => {
    elementCaptureService.candidates.mockResolvedValue([candidate(1, { name: "登录按钮" })]);
    elementCaptureService.update.mockRejectedValueOnce(new Error("名称冲突"));
    render(<ElementCaptureDrawer session={session} />);
    const input = await screen.findByLabelText("候选名称 1");

    fireEvent.change(input, { target: { value: "重复名称" } });
    fireEvent.blur(input);

    await waitFor(() => expect(input).toHaveValue("登录按钮"));
    expect(screen.getByText("名称冲突")).toBeInTheDocument();
  });

  it("409 issues 映射回候选并聚焦第一项", async () => {
    elementCaptureService.candidates.mockResolvedValue([candidate(1, { name: "登录按钮" })]);
    const error = new Error("批量预检失败");
    error.status = 409;
    error.issues = [{ candidateId: 1, field: "name", message: "页面内元素名称重复" }];
    elementCaptureService.save.mockRejectedValue(error);
    render(<ElementCaptureDrawer session={session} />);
    const row = await screen.findByTestId("element-capture-candidate-1");

    fireEvent.click(within(row).getByRole("checkbox", { name: "选择 登录按钮" }));
    fireEvent.click(screen.getByRole("button", { name: "保存选中元素" }));

    expect(await within(row).findByText("页面内元素名称重复")).toBeInTheDocument();
    expect(row).toHaveFocus();
  });

  it("批量保存提交 resolution/target，成功移除候选并调用 onSaved", async () => {
    elementCaptureService.candidates.mockResolvedValue([
      candidate(1),
      candidate(2, {
        conflictStatus: "duplicate",
        duplicateElementId: 42,
        conflictTargetIds: [42, 43],
        conflictResolution: "update",
        targetElementId: 43
      })
    ]);
    const result = { savedCandidateIds: [1, 2], ignoredCandidateIds: [] };
    elementCaptureService.save.mockResolvedValue(result);
    const onSaved = vi.fn();
    render(<ElementCaptureDrawer session={session} onSaved={onSaved} />);
    const first = await screen.findByTestId("element-capture-candidate-1");
    const second = screen.getByTestId("element-capture-candidate-2");

    fireEvent.click(within(first).getByRole("checkbox"));
    fireEvent.click(within(second).getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "保存选中元素" }));

    await waitFor(() => expect(elementCaptureService.save).toHaveBeenCalledWith(
      "session-1",
      [
        { candidateId: 1, resolution: "create" },
        { candidateId: 2, resolution: "update", targetElementId: 43 }
      ],
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    ));
    await waitFor(() => expect(screen.queryByTestId("element-capture-candidate-1")).not.toBeInTheDocument());
    expect(onSaved).toHaveBeenCalledWith(result);
  });

  it("支持 mode/stop/断连提示且停止不会清空未保存候选", async () => {
    elementCaptureService.get.mockResolvedValue({ ...session, status: "interrupted" });
    elementCaptureService.candidates.mockResolvedValue([candidate(1)]);
    render(<ElementCaptureDrawer session={session} />);
    const row = await screen.findByTestId("element-capture-candidate-1");

    expect(screen.getByRole("status")).toHaveTextContent("执行器连接中断");
    fireEvent.click(screen.getByRole("button", { name: "操作模式" }));
    await waitFor(() => expect(elementCaptureService.mode).toHaveBeenCalledWith(
      "session-1",
      "operate",
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    ));
    fireEvent.click(within(row).getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "停止采集" }));
    await waitFor(() => expect(elementCaptureService.stop).toHaveBeenCalled());
    expect(screen.getByTestId("element-capture-candidate-1")).toBeInTheDocument();
    expect(screen.getByText("会话已停止，候选仍需保存或忽略。")).toBeInTheDocument();
  });

  it("未保存关闭需确认，dialog 焦点可返回且不渲染敏感字段", async () => {
    const opener = document.createElement("button");
    opener.textContent = "打开审核";
    document.body.appendChild(opener);
    opener.focus();
    elementCaptureService.candidates.mockResolvedValue([candidate(1)]);
    const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
    const onClose = vi.fn();
    render(<ElementCaptureDrawer session={session} onClose={onClose} />);
    const row = await screen.findByTestId("element-capture-candidate-1");

    expect(screen.getByRole("dialog", { name: "候选元素审核" })).toHaveAttribute("aria-modal", "true");
    expect(screen.getByRole("button", { name: "关闭候选审核" })).toHaveFocus();
    expect(document.body.textContent).not.toContain("session-secret");
    expect(document.body.textContent).not.toContain("session-token");
    expect(document.body.textContent).not.toContain("command-receipt");
    expect(document.body.textContent).not.toContain("fingerprint-secret");
    fireEvent.click(within(row).getByRole("checkbox"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(confirm).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "关闭候选审核" }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(opener).toHaveFocus();
    opener.remove();
  });
});
