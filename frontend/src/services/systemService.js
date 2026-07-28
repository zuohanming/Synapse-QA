import { API_BASE } from "../config/appConfig.js";
import { getToken, request, toQuery } from "./httpClient.js";

export const systemService = {
  overview: () => request("/system/overview"),
  users: (params = {}) => request(`/system/users${toQuery(params)}`),
  createUser: (body) => request("/system/users", { method: "POST", body: JSON.stringify(body) }),
  roles: () => request("/system/roles"),
  permissions: () => request("/system/permissions"),
  menus: () => request("/system/menus"),
  dictionaries: () => request("/system/dictionaries"),
  logs: (params = {}) => request(`/system/logs${toQuery(params)}`),
  updateUser: (id, body) => request(`/system/users/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  resetUserPassword: (id) => request(`/system/users/${id}/reset-password`, { method: "POST" }),
  unlockUser: (id) => request(`/system/users/${id}/unlock`, { method: "POST" }),
  deleteUser: (id) => request(`/system/users/${id}`, { method: "DELETE" }),
  createRole: (body) => request("/system/roles", { method: "POST", body: JSON.stringify(body) }),
  updateRole: (id, body) => request(`/system/roles/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteRole: (id) => request(`/system/roles/${id}`, { method: "DELETE" }),
  settings: () => request("/system/settings"),
  updateSettings: (groupKey, body) => request(`/system/settings/${groupKey}`, { method: "PATCH", body: JSON.stringify(body) }),
  settingHistory: (groupKey) => request(`/system/settings/${groupKey}/history`),
  rollbackSettings: (groupKey, body) => request(`/system/settings/${groupKey}/rollback`, { method: "POST", body: JSON.stringify(body) })
};

export async function downloadOperationLogs(params = {}) {
  const response = await fetch(`${API_BASE}/system/logs/export${toQuery(params)}`, {
    headers: { Authorization: `Bearer ${getToken()}` }
  });
  if (!response.ok) throw new Error("导出操作日志失败");
  const url = URL.createObjectURL(await response.blob());
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = "operation-logs.csv";
  anchor.click();
  URL.revokeObjectURL(url);
}
