import { request, toQuery } from "./httpClient.js";

export const executionService = {
  executors: () => request("/executors"),
  createExecutor: (body) => request("/executors", { method: "POST", body: JSON.stringify(body) }),
  generateExecutorToken: (executorId) => request(`/executors/${encodeURIComponent(executorId)}/token`, { method: "POST" }),
  list: (params = {}) => request(`/executions${toQuery(params)}`),
  statistics: () => request("/executions/statistics"),
  get: (id) => request(`/executions/${id}`),
  create: (body) => request("/executions", { method: "POST", body: JSON.stringify(body) }),
  debug: (body) => request("/executions/debug", { method: "POST", body: JSON.stringify(body) }),
  debugStatus: (taskId, executorId) => request(`/executions/debug/${taskId}${toQuery({ executorId })}`),
  cancel: (id) => request(`/executions/${id}/cancel`, { method: "POST" }),
  logs: (taskId) => request(`/executions/tasks/${taskId}/logs`)
};
