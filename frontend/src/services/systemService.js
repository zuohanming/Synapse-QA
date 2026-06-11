import { request, toQuery } from "./httpClient.js";

export const systemService = {
  overview: () => request("/system/overview"),
  users: (params = {}) => request(`/system/users${toQuery(params)}`),
  roles: () => request("/system/roles"),
  menus: () => request("/system/menus"),
  dictionaries: () => request("/system/dictionaries"),
  logs: () => request("/system/logs"),
  updateUser: (id, body) => request(`/system/users/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteUser: (id) => request(`/system/users/${id}`, { method: "DELETE" }),
  createRole: (body) => request("/system/roles", { method: "POST", body: JSON.stringify(body) }),
  updateRole: (id, body) => request(`/system/roles/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteRole: (id) => request(`/system/roles/${id}`, { method: "DELETE" })
};
