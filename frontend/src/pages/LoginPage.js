import { useState } from "react";
import { ArrowRight, LockKeyhole, UserRound } from "lucide-react";
import { useAuth } from "../hooks/useAuth.js";

export function LoginPage() {
  const { login } = useAuth();
  const [form, setForm] = useState({ username: "admin", password: "admin123" });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    setBusy(true);
    try {
      await login(form);
    } catch (err) {
      setError(err.message || "登录失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login-view">
      <div className="login-grid" aria-hidden="true" />
      <section className="login-shell">
        <header className="login-brand"><div className="login-logo"><span /><i /></div><div><strong>Synapse QA</strong><small>QUALITY ENGINEERING</small></div></header>
        <form className="login-card" onSubmit={handleSubmit}>
          <div className="login-card-heading"><span>平台登录</span><h1>继续进入工作台</h1><p>管理测试资产、执行任务与质量报告</p></div>
          <label><span>用户名</span><div className="login-input"><UserRound size={16} /><input aria-label="用户名" autoComplete="username" placeholder="请输入用户名" value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} /></div></label>
          <label><span>密码</span><div className="login-input"><LockKeyhole size={16} /><input aria-label="密码" autoComplete="current-password" placeholder="请输入密码" type="password" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /></div></label>
          {error ? <div className="form-error" role="alert">{error}</div> : null}
          <button className="primary-button login-submit" disabled={busy} type="submit"><span>{busy ? "正在验证" : "登录"}</span><ArrowRight size={16} /></button>
          <div className="login-demo-account"><span>演示账号</span><code>admin / admin123</code></div>
        </form>
        <footer className="login-footer"><span>Synapse QA</span><i />自动化质量管理平台</footer>
      </section>
    </main>
  );
}
