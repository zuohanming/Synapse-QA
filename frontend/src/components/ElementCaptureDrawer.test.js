import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  classifyPollError,
  ElementCaptureDrawer,
  getPollDelay,
  getSaveBlockers,
  mergeCaptureCandidates,
  shouldContinueCandidatePaging,
  validateSaveResult
} from "./ElementCaptureDrawer.js";
import { elementCaptureService } from "../services/elementCaptureService.js";

vi.mock("../services/elementCaptureService.js", () => ({
  elementCaptureService: {
    get: vi.fn(),
    candidates: vi.fn(),
    update: vi.fn(),
    save: vi.fn(),
    remove: vi.fn(),
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
    elementCaptureService.remove.mockResolvedValue({ message: "候选项已删除" });
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
    expect([0, 1, 2, 3, 4, 5].map(getPollDelay)).toEqual([2000, 2000, 4000, 8000, 16000, 30000]);
  });

  it.each([400, 500])("终态继续分页直到接收完 %i 个候选", async (total) => {
    elementCaptureService.get.mockResolvedValue({ ...session, status: "completed", candidateCount: total });
    elementCaptureService.candidates.mockImplementation((_, { afterId, limit }) => Promise.resolve(
      Array.from(
        { length: Math.min(limit, total - afterId) },
        (_, index) => candidate(afterId + index + 1)
      )
    ));
    render(<ElementCaptureDrawer session={session} />);

    await waitFor(() => expect(elementCaptureService.candidates).toHaveBeenCalledTimes(total / 100));
    expect(elementCaptureService.candidates.mock.calls.at(-1)[1].afterId).toBe(total - 100);
  });

  it("分页判定同时支持短页和服务端总数", () => {
    expect(shouldContinueCandidatePaging(100, 100, 500, 100)).toBe(true);
    expect(shouldContinueCandidatePaging(100, 500, 500, 100)).toBe(false);
    expect(shouldContinueCandidatePaging(99, 99, 500, 100)).toBe(false);
  });

  it("区分网络、5xx、认证授权、会话缺失和业务错误", () => {
    expect(classifyPollError(new Error("network"))).toMatchObject({ retryable: true });
    expect(classifyPollError(Object.assign(new Error("busy"), { status: 503 }))).toMatchObject({ retryable: true });
    expect(classifyPollError(Object.assign(new Error("unauthorized"), { status: 401 }))).toMatchObject({
      retryable: false,
      message: expect.stringContaining("登录")
    });
    expect(classifyPollError(Object.assign(new Error("forbidden"), { status: 403 }))).toMatchObject({
      retryable: false,
      message: expect.stringContaining("权限")
    });
    expect(classifyPollError(Object.assign(new Error("missing"), { status: 404 }))).toMatchObject({
      retryable: false,
      message: expect.stringContaining("不存在")
    });
    expect(classifyPollError(Object.assign(new Error("业务校验失败"), { status: 400 }))).toEqual({
      retryable: false,
      message: "业务校验失败"
    });
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

  it("断网按首次 2 秒节奏退避且保留已有候选", async () => {
    vi.useFakeTimers();
    elementCaptureService.get
      .mockResolvedValueOnce(session)
      .mockResolvedValueOnce(session)
      .mockResolvedValueOnce({ ...session, status: "completed", candidateCount: 1 });
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
      vi.advanceTimersByTime(1999);
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
  });

  it("质量轨道计算数量并过滤候选，同时展示 400/500 容量提示", async () => {
    elementCaptureService.get.mockResolvedValue({ ...session, candidateCount: 500 });
    elementCaptureService.candidates.mockResolvedValue([
      candidate(1, { name: "可保存" }),
      candidate(2, { name: "未命名元素" }),
      candidate(3, { name: "弱定位", locators: [{ type: "css", value: ".item", score: 65, unique: false }] }),
      candidate(4, { name: "重复元素", conflictStatus: "duplicate", duplicateElementId: 42 }),
      candidate(5, {
        name: "未命名元素",
        locators: [],
        conflictStatus: "duplicate",
        conflictResolution: "ignore",
        conflictTargets: [{ id: 42, name: "提交" }]
      })
    ]);
    render(<ElementCaptureDrawer session={session} />);

    expect(await screen.findByRole("button", { name: "可保存 2" })).toBeInTheDocument();
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

  it("删除候选会调用接口并从待审核列表移除", async () => {
    elementCaptureService.candidates.mockResolvedValue([candidate(1)]);
    render(<ElementCaptureDrawer session={session} />);

    const row = await screen.findByTestId("element-capture-candidate-1");
    fireEvent.click(within(row).getByRole("button", { name: "删除" }));

    await waitFor(() => expect(elementCaptureService.remove).toHaveBeenCalledWith(
      "session-1", 1, expect.objectContaining({ signal: expect.any(AbortSignal) })
    ));
    expect(screen.queryByTestId("element-capture-candidate-1")).not.toBeInTheDocument();
  });

  it("保存门禁覆盖未命名、可靠唯一定位、冲突 target、200 上限和 400 预警", () => {
    expect(getSaveBlockers([candidate(1, { name: "未命名元素" })], new Set([1]), "active")).toContain("1 个候选尚未命名");
    expect(getSaveBlockers([
      candidate(1, { locators: [{ type: "css", value: ".item", score: 95, unique: false }] })
    ], new Set([1]), "active")).toContain("1 个候选缺少评分不低于 40 的唯一定位器");
    expect(getSaveBlockers([
      candidate(1, { locators: [{ type: "xpath", value: "//input[@placeholder='用户名']", score: 45, unique: true }] })
    ], new Set([1]), "active")).toEqual([]);
    expect(getSaveBlockers([
      candidate(1, { conflictStatus: "duplicate", duplicateElementId: 42 })
    ], new Set([1]), "active")).toContain("1 个候选尚未处理冲突");
    expect(getSaveBlockers([
      candidate(1, {
        conflictStatus: "duplicate",
        conflictResolution: "update",
        conflictTargets: [{ id: 42, name: "提交" }, { id: 43, name: "提交副本" }],
        targetElementId: 0
      })
    ], new Set([1]), "active")).toContain("1 个更新候选需要选择目标元素");
    const twoHundredOne = Array.from({ length: 201 }, (_, index) => candidate(index + 1));
    expect(getSaveBlockers(twoHundredOne, new Set(twoHundredOne.map((item) => item.cursorId)), "active")).toContain("一次最多保存 200 个候选");
    expect(getSaveBlockers([candidate(1)], new Set([1]), "failed")).toContain("当前会话状态不允许保存");
    expect(getSaveBlockers([candidate(1)], new Set([1]), "completed")).toEqual([]);
    expect(getSaveBlockers([
      candidate(1, {
        name: "未命名元素",
        locators: [],
        conflictStatus: "duplicate",
        conflictResolution: "ignore",
        conflictTargets: [{ id: 42, name: "提交" }, { id: 43, name: "副本" }]
      })
    ], new Set([1]), "active")).toEqual([]);
  });

  it("冲突支持 update/ignore/create，多目标 update 必须选择 target", async () => {
    elementCaptureService.candidates.mockResolvedValue([
      candidate(4, {
        name: "重复元素",
        conflictStatus: "duplicate",
        duplicateElementId: 42,
        conflictTargets: [{ id: 42, name: "提交" }, { id: 43, name: "提交副本" }]
      })
    ]);
    render(<ElementCaptureDrawer session={session} />);
    const row = await screen.findByTestId("element-capture-candidate-4");

    fireEvent.click(within(row).getByRole("checkbox"));
    fireEvent.click(within(row).getByRole("button", { name: "更新已有元素" }));
    expect(within(row).getByRole("button", { name: "更新已有元素" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("1 个更新候选需要选择目标元素")).toBeInTheDocument();
    fireEvent.change(within(row).getByLabelText("更新目标"), { target: { value: "43" } });
    expect(within(row).getByRole("option", { name: "提交副本 (#43)" })).toBeInTheDocument();
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
        conflictTargets: [{ id: 42, name: "提交" }, { id: 43, name: "提交副本" }],
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

  it("保存响应必须完整且只能包含本次提交的候选 ID", () => {
    const selected = [candidate(1), candidate(2, { conflictStatus: "duplicate", conflictResolution: "ignore" })];
    expect(validateSaveResult(
      { savedCandidateIds: [1], ignoredCandidateIds: [2] },
      selected
    )).toEqual(new Set([1, 2]));
    expect(() => validateSaveResult(
      { savedCandidateIds: [1], ignoredCandidateIds: [] },
      selected
    )).toThrow("不完整");
    expect(() => validateSaveResult(
      { savedCandidateIds: [1, 3], ignoredCandidateIds: [2] },
      selected
    )).toThrow("未知");
  });

  it("保存响应缺少 ID 时保留候选和选择并显示错误", async () => {
    elementCaptureService.candidates.mockResolvedValue([candidate(1)]);
    elementCaptureService.save.mockResolvedValue({ savedCandidateIds: [], ignoredCandidateIds: [] });
    const onSaved = vi.fn();
    render(<ElementCaptureDrawer session={session} onSaved={onSaved} />);
    const row = await screen.findByTestId("element-capture-candidate-1");

    fireEvent.click(within(row).getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "保存选中元素" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("保存响应不完整");
    expect(screen.getByTestId("element-capture-candidate-1")).toBeInTheDocument();
    expect(within(row).getByRole("checkbox")).toBeChecked();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("旧轮询详情不会覆盖用户刚切换的模式", async () => {
    const staleDetail = deferred();
    elementCaptureService.get.mockReturnValue(staleDetail.promise);
    elementCaptureService.candidates.mockResolvedValue([]);
    render(<ElementCaptureDrawer session={session} />);

    fireEvent.click(screen.getByRole("button", { name: "操作模式" }));
    await waitFor(() => expect(elementCaptureService.mode).toHaveBeenCalled());
    await act(async () => staleDetail.resolve({ ...session, mode: "pick" }));

    expect(screen.getByRole("button", { name: "操作模式" })).toHaveAttribute("aria-pressed", "true");
  });

  it("401 轮询错误不可重试且不显示断网退避", async () => {
    vi.useFakeTimers();
    const error = new Error("登录已过期");
    error.status = 401;
    elementCaptureService.get.mockRejectedValue(error);
    render(<ElementCaptureDrawer session={session} />);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByRole("alert")).toHaveTextContent("登录");
    expect(screen.queryByText(/秒后重试/)).not.toBeInTheDocument();
    await act(async () => {
      vi.advanceTimersByTime(30000);
      await Promise.resolve();
    });
    expect(elementCaptureService.get).toHaveBeenCalledTimes(1);
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
    await waitFor(() => expect(elementCaptureService.stop).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    ));
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(opener).toHaveFocus();
    opener.remove();
  });
});
