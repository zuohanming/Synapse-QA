import { request, getToken } from "./httpClient.js";
import { API_BASE } from "../config/appConfig.js";

export const aiService = {
  getConfig: () => request("/ai/config"),
  saveModel: (model) =>
    request("/ai/models", { method: "POST", body: JSON.stringify(model) }),
  deleteModel: (id) =>
    request(`/ai/models/${encodeURIComponent(id)}`, { method: "DELETE" }),
  testConnection: (provider, apiKey, baseUrl) =>
    request("/ai/test-connection", {
      method: "POST",
      body: JSON.stringify({ provider, apiKey, baseUrl: baseUrl || undefined }),
    }),

  savePreferences: (preferences) =>
    request("/ai/preferences", { method: "POST", body: JSON.stringify(preferences) }),

  async *chatStream(messages) {
    const token = getToken();
    const resp = await fetch(`${API_BASE}/ai/chat`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: token ? `Bearer ${token}` : "",
      },
      body: JSON.stringify({ messages }),
    });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({}));
      throw new Error(err.error || "AI 请求失败");
    }
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";
      for (const line of lines) {
        if (line.startsWith("data: ")) {
          const data = line.slice(6);
          try {
            yield JSON.parse(data);
          } catch {
            // skip malformed
          }
        }
      }
    }
  },
};
