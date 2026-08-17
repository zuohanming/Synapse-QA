import { request, toQuery } from "./httpClient.js";

// 性能测试服务层。字段命名与 SPEC v2.5 契约一致：
// scenarioType / loadConfig / environment / thresholds(数组) / planSnapshot / idempotencyKey 等。
export const performanceService = {
  plans: {
    list: (params = {}) => request(`/perf/plans${toQuery(params)}`),
    get: (id) => request(`/perf/plans/${id}`),
    create: (body) => request("/perf/plans", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/perf/plans/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`/perf/plans/${id}`, { method: "DELETE" }),
    // 触发执行：idempotencyKey 由前端生成且永久唯一（SPEC §5.3）。
    run: (id, idempotencyKey) => request(`/perf/plans/${id}/run`, { method: "POST", body: JSON.stringify({ idempotencyKey }) })
  },
  runs: {
    list: (params = {}) => request(`/perf/runs${toQuery(params)}`),
    get: (id) => request(`/perf/runs/${id}`),
    cancel: (id) => request(`/perf/runs/${id}/cancel`, { method: "POST" })
  },
  // 冒烟测试请求（SPEC §8.5）：通过所选执行器发送一次请求，返回 taskId 后轮询，不创建正式 perf_test_runs。
  smoke: {
    start: (body = {}) => request("/perf/smoke", { method: "POST", body: JSON.stringify(body) }),
    status: (taskId) => request(`/perf/smoke/${taskId}`)
  },
  executors: () => request("/executors")
};
