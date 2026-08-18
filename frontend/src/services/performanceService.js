import { API_BASE, TOKEN_KEY } from "../config/appConfig.js";
import { getToken, request, toQuery } from "./httpClient.js";

function parseSseEvent(block) {
  const lines = block.split(/\r?\n/);
  let event = "message";
  let id = "";
  const data = [];
  lines.forEach((line) => {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("id:")) id = line.slice(3).trim();
    else if (line.startsWith("data:")) data.push(line.slice(5).replace(/^ /, ""));
  });
  if (!data.length) return null;
  let value = data.join("\n");
  try { value = JSON.parse(value); } catch { /* 保留文本消息 */ }
  return { event, id, data: value };
}

export function parseSseChunk(buffer, chunk) {
  const text = `${buffer || ""}${chunk || ""}`;
  const blocks = text.split(/\r?\n\r?\n/);
  return { events: blocks.slice(0, -1).map(parseSseEvent).filter(Boolean), buffer: blocks.at(-1) || "" };
}

// 性能测试服务层。字段命名与 SPEC v2.5 契约一致：
// scenarioType / loadConfig / environment / thresholds(数组) / planSnapshot / idempotencyKey 等。
export const performanceService = {
  environments: (productId) => request(`/perf/environments${toQuery({ productId })}`),
  plans: {
    list: (params = {}) => request(`/perf/plans${toQuery(params)}`),
    get: (id) => request(`/perf/plans/${id}`),
    create: (body) => request("/perf/plans", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/perf/plans/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`/perf/plans/${id}`, { method: "DELETE" }),
    // 触发执行：idempotencyKey 由前端生成且永久唯一（SPEC §5.3）。
    run: (id, idempotencyKey, overrides = {}) => request(`/perf/plans/${id}/run`, { method: "POST", body: JSON.stringify({ idempotencyKey, ...overrides }) })
  },
  runs: {
    list: (params = {}) => request(`/perf/runs${toQuery(params)}`),
    get: (id) => request(`/perf/runs/${id}`),
    cancel: (id) => request(`/perf/runs/${id}/cancel`, { method: "POST" })
    ,stream: (id, after, handlers = {}) => {
      const controller = new AbortController();
      const token = getToken() || localStorage.getItem(TOKEN_KEY) || "";
      const query = toQuery({ after });
      controller.promise = (async () => {
        const response = await fetch(`${API_BASE}/perf/runs/${encodeURIComponent(id)}/events/stream${query}`, {
          headers: { Accept: "text/event-stream", Authorization: `Bearer ${token}`, "Last-Event-ID": String(after ?? "") },
          signal: controller.signal
        });
        if (!response.ok) throw new Error("监控连接失败");
        if (!response.body) throw new Error("浏览器不支持实时监控流");
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        while (!controller.signal.aborted) {
          const result = await reader.read();
          if (result.done) break;
          const parsed = parseSseChunk(buffer, decoder.decode(result.value, { stream: true }));
          buffer = parsed.buffer;
          for (const { event, id: eventId, data } of parsed.events) {
            const handler = handlers[event] || handlers.onEvent;
            if (handler) await handler(data, eventId);
            if (event === "perf.terminal") {
              controller.abort();
              return;
            }
          }
        }
      })();
      controller.cancel = () => controller.abort();
      return controller;
    },
    baseline: {
      get: (id) => request(`/perf/runs/${id}/baseline`),
      set: (id) => request(`/perf/runs/${id}/baseline`, { method: "PUT" }),
      remove: (id) => request(`/perf/runs/${id}/baseline`, { method: "DELETE" })
    },
    compare: (id) => request(`/perf/runs/${id}/compare`),
    trend: (id, limit = 20) => request(`/perf/runs/${id}/trend${toQuery({ limit })}`),
    export: async (id, format) => {
      const response = await fetch(`${API_BASE}/perf/runs/${encodeURIComponent(id)}/export${toQuery({ format })}`, { headers: { Authorization: `Bearer ${getToken() || ""}` } });
      if (!response.ok) { const payload = await response.json().catch(() => ({})); throw new Error(payload.error || "导出失败"); }
      const blob = await response.blob();
      const disposition = response.headers.get("Content-Disposition") || "";
      const match = /filename\*?=(?:UTF-8''|\")?([^;\"]+)/i.exec(disposition);
      return { blob, filename: match ? decodeURIComponent(match[1].trim()) : `perf-run-${id}.${format}` };
    }
  },
  schedules: {
    list: (params = {}) => request(`/perf/schedules${toQuery(params)}`),
    create: (body) => request("/perf/schedules", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/perf/schedules/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`/perf/schedules/${id}`, { method: "DELETE" }),
    enable: (id) => request(`/perf/schedules/${id}/enable`, { method: "POST" }),
    disable: (id) => request(`/perf/schedules/${id}/disable`, { method: "POST" })
  },
  // 冒烟测试请求（SPEC §8.5）：通过所选执行器发送一次请求，返回 taskId 后轮询，不创建正式 perf_test_runs。
  smoke: {
    start: (body = {}) => request("/perf/smoke", { method: "POST", body: JSON.stringify(body) }),
    status: (taskId) => request(`/perf/smoke/${taskId}`),
    cancel: (taskId) => request(`/perf/smoke/${taskId}/cancel`, { method: "POST" })
  },
  executors: () => request("/executors")
};
