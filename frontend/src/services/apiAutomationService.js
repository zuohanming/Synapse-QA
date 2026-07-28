import { API_BASE } from "../config/appConfig.js";
import { getToken, request, toQuery } from "./httpClient.js";

async function streamDebugEvents(taskId, after, onEvent, signal) {
  const response = await fetch(`${API_BASE}/api-automation/debug/${encodeURIComponent(taskId)}/events/stream${toQuery({ after })}`, {
    headers: { Authorization: `Bearer ${getToken()}` },
    signal
  });
  if (!response.ok || !response.body) throw new Error("连接实时事件流失败");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done }).replace(/\r\n/g, "\n");
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      const data = frame.split("\n").filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n");
      if (data) onEvent(JSON.parse(data));
      boundary = buffer.indexOf("\n\n");
    }
    if (done) return;
  }
}

export const apiAutomationService = {
  interfaces: {
    list: (params = {}) => request(`/api-automation/interfaces${toQuery(params)}`),
    get: (id) => request(`/api-automation/interfaces/${id}`),
    preview: (id, body) => request(`/api-automation/interfaces/${id}/preview`, { method: "POST", body: JSON.stringify(body) }),
    exportCurl: (id, body) => request(`/api-automation/interfaces/${id}/curl`, { method: "POST", body: JSON.stringify(body) }),
    create: (body) => request("/api-automation/interfaces", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/api-automation/interfaces/${id}`, { method: "PATCH", headers: { "If-Match": String(body.revision || "") }, body: JSON.stringify(body) }),
    saveConfiguration: (id, body) => request(`/api-automation/interfaces/${id}/configuration`, { method: "PATCH", headers: { "If-Match": String(body.revision || "") }, body: JSON.stringify(body) }),
    remove: (id) => request(`/api-automation/interfaces/${id}`, { method: "DELETE" }),
    restore: (id) => request(`/api-automation/interfaces/${id}/restore`, { method: "POST" }),
    batchDelete: (ids) => request("/api-automation/interfaces/batch-delete", { method: "POST", body: JSON.stringify({ ids }) }),
    batchStatus: (ids, status) => request("/api-automation/interfaces/batch-status", { method: "POST", body: JSON.stringify({ ids, status }) }),
    batchMove: (ids, productId, moduleId = 0) => request("/api-automation/interfaces/batch-move", { method: "POST", body: JSON.stringify({ ids, productId, moduleId }) })
  },
  curl: {
    parse: (curl) => request("/api-automation/curl/parse", { method: "POST", body: JSON.stringify({ curl }) })
  },
  tempFiles: {
    upload: (projectId, file) => {
      const body = new FormData();
      body.append("projectId", String(projectId));
      body.append("file", file);
      return request("/api-automation/temp-files", { method: "POST", body });
    },
    remove: (id) => request(`/api-automation/temp-files/${encodeURIComponent(id)}`, { method: "DELETE" })
  },
  debug: {
    start: (id, body) => request(`/api-automation/interfaces/${id}/debug`, { method: "POST", body: JSON.stringify(body) }),
    get: (taskId) => request(`/api-automation/debug/${encodeURIComponent(taskId)}`),
    events: (taskId, after = 0) => request(`/api-automation/debug/${encodeURIComponent(taskId)}/events${toQuery({ after })}`),
    stream: streamDebugEvents,
    cancel: (taskId) => request(`/api-automation/debug/${encodeURIComponent(taskId)}/cancel`, { method: "POST" }),
    history: (interfaceId, params = {}) => request(`/api-automation/interfaces/${interfaceId}/debug-runs${toQuery(params)}`),
    historyDetail: (id) => request(`/api-automation/debug-runs/${id}`)
  },
  versions: {
    list: (interfaceId) => request(`/api-automation/interfaces/${interfaceId}/versions`),
    get: (interfaceId, version) => request(`/api-automation/interfaces/${interfaceId}/versions/${version}`),
    diff: (interfaceId, version, targetVersion) => request(`/api-automation/interfaces/${interfaceId}/versions/${version}/diff${toQuery({ targetVersion })}`),
    restore: (interfaceId, version) => request(`/api-automation/interfaces/${interfaceId}/versions/${version}/restore`, { method: "POST" })
  },
  requestHeaders: {
    list: (params = {}) => request(`/api-automation/project-headers${toQuery(params)}`),
    create: (body) => request("/api-automation/project-headers", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`/api-automation/project-headers/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`/api-automation/project-headers/${id}`, { method: "DELETE" })
  }
};
