import { API_BASE } from "../config/appConfig.js";

function authHeaders() {
  const token = localStorage.getItem("synapse_qa_token");
  return {
    "Content-Type": "application/json",
    ...(token ? { Authorization: `Bearer ${token}` } : {})
  };
}

async function request(path, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, { headers: authHeaders(), ...options });
  return res.json();
}

export const dataFactoryService = {
  listGenerators() {
    return request("/data-factory/generators");
  },
  preview(placeholder, count = 5) {
    return request("/data-factory/generators/preview", {
      method: "POST",
      body: JSON.stringify({ placeholder, count })
    });
  }
};
