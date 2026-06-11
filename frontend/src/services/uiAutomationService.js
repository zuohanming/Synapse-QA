import { request, toQuery } from "./httpClient.js";

function uiResource(path) {
  return {
    list: (params = {}) => request(`${path}${toQuery(params)}`),
    create: (body) => request(path, { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`${path}/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`${path}/${id}`, { method: "DELETE" })
  };
}

export const uiAutomationService = {
  elements: uiResource("/ui/elements"),
  steps: uiResource("/ui/steps"),
  cases: uiResource("/ui/cases"),
  variables: uiResource("/ui/variables"),
  pageElements: uiResource("/ui/page-elements")
};
