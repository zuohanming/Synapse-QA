import { request, toQuery } from "./httpClient.js";

async function interfaceRequest(primaryPath, legacyPath, options) {
  try {
    return await request(primaryPath, options);
  } catch (error) {
    if (error.status !== 404) throw error;
    return request(legacyPath, options);
  }
}

export const apiAutomationService = {
  interfaces: {
    list: (params = {}) => interfaceRequest(`/interfaces${toQuery(params)}`, `/ui/cases${toQuery(params)}`),
    create: (body) => interfaceRequest("/interfaces", "/ui/cases", { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => interfaceRequest(`/interfaces/${id}`, `/ui/cases/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => interfaceRequest(`/interfaces/${id}`, `/ui/cases/${id}`, { method: "DELETE" })
  }
};
