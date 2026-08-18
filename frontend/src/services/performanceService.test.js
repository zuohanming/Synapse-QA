import { afterEach, describe, expect, it, vi } from "vitest";
import { parseSseChunk, performanceService } from "./performanceService.js";

describe("性能监控 SSE", () => {
  afterEach(() => vi.restoreAllMocks());

  it("支持跨 chunk 分帧、event/id/data 和多事件", () => {
    const first = parseSseChunk("", "id: 1\nevent: perf.sample\ndata: {\"sequence\":1");
    expect(first.events).toEqual([]);
    const second = parseSseChunk(first.buffer, "}\n\nevent: perf.terminal\ndata: {\"status\":\"completed\"}\n\n");
    expect(second.events[0]).toEqual({ event: "perf.sample", id: "1", data: { sequence: 1 } });
    expect(second.events[1]).toEqual({ event: "perf.terminal", id: "", data: { status: "completed" } });
  });

  it("按顺序等待 handler，terminal 后 abort 且不继续处理后续事件", async () => {
    const encoder = new TextEncoder();
    const body = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode("event: perf.sample\ndata: {\"sequence\":1}\n\nevent: perf.terminal\ndata: {}\n\nevent: perf.sample\ndata: {\"sequence\":2}\n\n"));
        controller.close();
      }
    });
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ ok: true, body })));
    const seen = [];
    const controller = performanceService.runs.stream("run-1", 0, {
      "perf.sample": async (sample) => { seen.push(sample.sequence); await Promise.resolve(); },
      "perf.terminal": () => seen.push("terminal")
    });
    await controller.promise;
    expect(seen).toEqual([1, "terminal"]);
    expect(controller.signal.aborted).toBe(true);
  });
});
