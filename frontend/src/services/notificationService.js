import { request, toQuery } from "./httpClient.js";

export const notificationService = {
  list: (params = {}) => request(`/notifications${toQuery(params)}`),
  unreadCount: () => request("/notifications/unread-count"),
  markRead: (id) => request(`/notifications/${id}/read`, { method: "PATCH" }),
  markAllRead: () => request("/notifications/read-all", { method: "POST" }),
  remove: (id) => request(`/notifications/${id}`, { method: "DELETE" }),
  preferences: () => request("/notifications/preferences"),
  updatePreferences: (body) => request("/notifications/preferences", { method: "PATCH", body: JSON.stringify(body) })
};
