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
      <section className="login-intro">
        <div className="login-brand"><div className="brand-mark"><span /></div><strong>Synapse QA</strong></div>
        <div className="login-message">
          <span className="login-eyebrow">QUALITY ENGINEERING PLATFORM</span>
          <h1>让每一次发布，<br />都有迹可循。</h1>
          <p>统一管理测试资产、执行链路与质量结果，让团队更快发现问题，更稳交付产品。</p>
          <div className="pulse-track" aria-hidden="true"><span /><i /><span /><i /><span /></div>
        </div>
        <small>Synapse QA · 自动化测试平台</small>
      </section>
      <section className="login-panel">
        <form className="login-card" onSubmit={handleSubmit}>
          <div className="login-card-heading"><span>欢迎回来</span><h2>登录工作台</h2><p>使用平台账号继续</p></div>
          <label><span>用户名</span><div className="login-input"><UserRound size={17} /><input autoComplete="username" value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} /></div></label>
          <label><span>密码</span><div className="login-input"><LockKeyhole size={17} /><input autoComplete="current-password" type="password" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /></div></label>
          {error ? <div className="form-error" role="alert">{error}</div> : null}
          <button className="primary-button login-submit" disabled={busy} type="submit"><span>{busy ? "正在登录" : "登录"}</span><ArrowRight size={17} /></button>
          <p className="login-hint">默认账号 admin · 密码 admin123</p>
        </form>
      </section>
    </main>
  );
}
