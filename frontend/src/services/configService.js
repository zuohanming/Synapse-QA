import { request, toQuery } from "./httpClient.js";

function resource(path) {
  return {
    list: (params = {}) => request(`${path}${toQuery(params)}`),
    create: (body) => request(path, { method: "POST", body: JSON.stringify(body) }),
    update: (id, body) => request(`${path}/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
    remove: (id) => request(`${path}/${id}`, { method: "DELETE" })
  };
}

export const configService = {
  projects: resource("/config/projects"),
  products: resource("/config/products"),
  testObjects: resource("/config/test-objects"),
  executorToken: {
    get: () => request("/config/executor-token"),
    generate: () => request("/config/executor-token/generate", { method: "POST" })
  }
};
