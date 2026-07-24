import { request, toQuery } from "./httpClient.js";

export const apiAutomationService = {
  interfaces: {
    list: (params = {}) => request(`/api-automation/interfaces${toQuery(params)}`),
    get: (id) => request(`/api-automation/interfaces/${id}`),
    create: (body) => request("/api-automation/interfaces", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/api-automation/interfaces/${id}`, { method: "PATCH", headers: { "If-Match": String(body.revision || "") }, body: JSON.stringify(body) }),
    remove: (id) => request(`/api-automation/interfaces/${id}`, { method: "DELETE" })
  },
  requestHeaders: {
    list: (params = {}) => request(`/api-automation/project-headers${toQuery(params)}`),
    create: (body) => request("/api-automation/project-headers", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/api-automation/project-headers/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`/api-automation/project-headers/${id}`, { method: "DELETE" })
  }
};
