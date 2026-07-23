import { request, toQuery } from "./httpClient.js";

function uiResource(path) {
  return {
    list: (params = {}) => request(`${path}${toQuery(params)}`),
    get: (id) => request(`${path}/${id}`),
    create: (body) => request(path, { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`${path}/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`${path}/${id}`, { method: "DELETE" })
  };
}

export const uiAutomationService = {
  elements: uiResource("/ui/elements"),
  steps: uiResource("/ui/steps"),
  cases: {
    ...uiResource("/test-cases"),
    import: (items) => request("/test-cases/import", { method: "POST", body: JSON.stringify({ items }) }),
    export: (params = {}) => request(`/test-cases/export${toQuery(params)}`),
    datasets: {
      list: (caseId) => request(`/test-cases/${caseId}/datasets`),
      create: (caseId, body) => request(`/test-cases/${caseId}/datasets`, { method: "POST", body: JSON.stringify(body) }),
      update: (caseId, datasetId, body) => request(`/test-cases/${caseId}/datasets/${datasetId}`, { method: "PATCH", body: JSON.stringify(body) }),
      remove: (caseId, datasetId) => request(`/test-cases/${caseId}/datasets/${datasetId}`, { method: "DELETE" })
    }
  },
  variables: uiResource("/ui/variables"),
  pageElements: uiResource("/ui/page-elements")
};
