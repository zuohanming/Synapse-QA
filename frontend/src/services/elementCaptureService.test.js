import { afterEach, describe, expect, it, vi } from "vitest";

import { elementCaptureService } from "./elementCaptureService.js";

function response(data, { ok = true, status = 200, error = "" } = {}) {
  return Promise.resolve({
    ok,
    status,
    json: () => Promise.resolve(ok ? { data } : { error, data })
  });
}

describe("elementCaptureService", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("复用 httpClient 调用完整采集 API 且不向调用方返回一次性 token", async () => {
    global.fetch = vi.fn(() => response({ session: { id: "session-1" }, token: "session-secret" }));

    const created = await elementCaptureService.create({
      pageId: 8,
      executorId: "exec-1",
      browserChannel: "chrome",
      mode: "pick",
      url: "https://example.test/login"
    });
    await elementCaptureService.get("session-1");
    await elementCaptureService.mode("session-1", "operate");
    await elementCaptureService.stop("session-1");
    await elementCaptureService.update("session-1", 12, { name: "登录按钮" });
    await elementCaptureService.save("session-1", [{ candidateId: 12, resolution: "create" }]);
    await elementCaptureService.versions(42);
    await elementCaptureService.rollback(42, 3);

    expect(created).toEqual({ id: "session-1" });
    expect(JSON.stringify(created)).not.toContain("session-secret");
    expect(global.fetch.mock.calls.map(([url]) => String(url))).toEqual([
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions",
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions/session-1",
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions/session-1/mode",
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions/session-1/stop",
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions/session-1/candidates/12",
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions/session-1/save",
      "http://127.0.0.1:8080/api/ui/page-elements/42/versions",
      "http://127.0.0.1:8080/api/ui/page-elements/42/versions/3/rollback"
    ]);
    expect(global.fetch.mock.calls.map(([, options]) => options.method || "GET")).toEqual([
      "POST", "GET", "PATCH", "POST", "PATCH", "POST", "GET", "POST"
    ]);
    expect(JSON.parse(global.fetch.mock.calls[2][1].body)).toEqual({ mode: "operate" });
    expect(JSON.parse(global.fetch.mock.calls[5][1].body)).toEqual({
      items: [{ candidateId: 12, resolution: "create" }]
    });
  });

  it("按游标查询候选并透传 AbortSignal", async () => {
    global.fetch = vi.fn(() => response([]));
    const controller = new AbortController();

    await elementCaptureService.candidates("session/1", {
      afterId: 12,
      limit: 200,
      signal: controller.signal
    });

    expect(global.fetch).toHaveBeenCalledWith(
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions/session%2F1/candidates?afterId=12&limit=200",
      expect.objectContaining({ signal: controller.signal })
    );
  });

  it("严格拒绝非整数游标、越界 limit 和非法资源 ID", async () => {
    global.fetch = vi.fn(() => response([]));

    expect(() => elementCaptureService.candidates("session-1", { afterId: 1.5 })).toThrow("afterId");
    expect(() => elementCaptureService.candidates("session-1", { afterId: "1" })).toThrow("afterId");
    expect(() => elementCaptureService.candidates("session-1", { limit: 201 })).toThrow("limit");
    expect(() => elementCaptureService.update("session-1", "12x", { name: "提交" })).toThrow("candidateId");
    expect(() => elementCaptureService.rollback(42, 0)).toThrow("version");
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("保存前在本地限制 1 到 200 个候选", () => {
    expect(() => elementCaptureService.save("session-1", [])).toThrow("1 到 200");
    const tooMany = Array.from({ length: 201 }, (_, index) => ({
      candidateId: index + 1,
      resolution: "create"
    }));
    expect(() => elementCaptureService.save("session-1", tooMany)).toThrow("1 到 200");
  });

  it("候选查询默认 afterId=0、limit=100", async () => {
    global.fetch = vi.fn(() => response([]));

    await elementCaptureService.candidates("session-1");

    expect(global.fetch.mock.calls[0][0]).toBe(
      "http://127.0.0.1:8080/api/ui/page-elements/capture-sessions/session-1/candidates?afterId=0&limit=100"
    );
  });

  it("保留 409 的结构化候选 issues", async () => {
    const issues = [{ candidateId: 12, field: "name", message: "名称重复" }];
    global.fetch = vi.fn(() => response({ issues }, { ok: false, status: 409, error: "候选项预检失败" }));

    await expect(
      elementCaptureService.save("session-1", [{ candidateId: 12, resolution: "create" }])
    ).rejects.toMatchObject({
      message: "候选项预检失败",
      status: 409,
      issues
    });
  });
});
