import { request, toQuery } from "./httpClient.js";

export const performanceService = {
  plans: {
    list: (params = {}) => request(`/perf/plans${toQuery(params)}`),
    get: (id) => request(`/perf/plans/${id}`),
    create: (body) => request("/perf/plans", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/perf/plans/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`/perf/plans/${id}`, { method: "DELETE" }),
    run: (id) => request(`/perf/plans/${id}/run`, { method: "POST" })
  },
  runs: {
    list: (params = {}) => request(`/perf/runs${toQuery(params)}`),
    get: (id) => request(`/perf/runs/${id}`)
  }
};
