import { request, toQuery } from "./httpClient.js";

export const executionService = {
  list: (params = {}) => request(`/executions${toQuery(params)}`),
  get: (id) => request(`/executions/${id}`),
  create: (body) => request("/executions", { method: "POST", body: JSON.stringify(body) }),
  cancel: (id) => request(`/executions/${id}/cancel`, { method: "POST" }),
  logs: (taskId) => request(`/executions/tasks/${taskId}/logs`)
};
