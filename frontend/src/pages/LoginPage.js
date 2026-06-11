import { useState } from "react";
import { useAuth } from "../hooks/useAuth.js";

export function LoginPage() {
  const { login } = useAuth();
  const [form, setForm] = useState({ username: "admin", password: "admin123" });
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    try {
      await login(form);
    } catch (err) {
      setError(err.message || "登录失败");
    }
  }

  return (
    <main className="login-view">
      <form className="login-card" onSubmit={handleSubmit}>
        <div className="login-mark" />
        <h1>Synapse QA</h1>
        <p>自动化测试平台</p>
        <label>
          <span>用户名</span>
          <input value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} />
        </label>
        <label>
          <span>密码</span>
          <input type="password" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} />
        </label>
        {error ? <div className="form-error">{error}</div> : null}
        <button className="primary-button" type="submit">登录</button>
      </form>
    </main>
  );
}
