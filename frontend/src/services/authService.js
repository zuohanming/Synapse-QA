import { request, setToken, clearToken } from "./httpClient.js";

export async function login(credentials) {
  const result = await request("/auth/login", {
    method: "POST",
    body: JSON.stringify(credentials)
  });
  setToken(result.token);
  return result.user;
}

export async function currentUser() {
  return request("/auth/me");
}

export function logout() {
  clearToken();
}
