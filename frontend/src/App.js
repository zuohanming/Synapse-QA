import { useEffect, useMemo, useState } from "react";
import { Layout } from "./components/Layout.js";
import { LoginPage } from "./pages/LoginPage.js";
import { routes } from "./routes/index.js";
import { useAuth } from "./hooks/useAuth.js";
import { cleanupRouteState, getDefaultPath, pathFromHash, persistRouteState, readRouteState, readScrollState } from "./utils/routeState.js";

export default function App() {
  const { user, bootstrapping, changePassword, logout } = useAuth();
  const [activePath, setActivePath] = useState(() => readRouteState());

  const activeRoute = useMemo(() => {
    return routes.find((route) => route.match(activePath)) || routes[0];
  }, [activePath]);

  useEffect(() => {
    cleanupRouteState();
  }, []);

  useEffect(() => {
    if (user) {
      persistRouteState(activePath);
    }
  }, [activePath, user]);

  useEffect(() => {
    if (!user) return undefined;
    const scroll = readScrollState();
    requestAnimationFrame(() => window.scrollTo(scroll.x || 0, scroll.y || 0));
    function handleBeforeUnload() {
      persistRouteState(activePath);
    }
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [activePath, user]);

  useEffect(() => {
    function handleHashChange() {
      const nextPath = pathFromHash();
      if (nextPath) {
        setActivePath(nextPath);
        return;
      }
      if (window.location.hash.replace(/^#\/?/, "")) {
        setActivePath(getDefaultPath());
      }
    }
    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, []);

  if (bootstrapping) {
    return <div className="screen-center">正在恢复登录状态</div>;
  }

  if (!user) {
    return <LoginPage />;
  }
  if (user.mustChangePassword) {
    return <RequiredPasswordChange user={user} onChange={changePassword} onLogout={logout} />;
  }

  return (
    <Layout activePath={activePath} onNavigate={setActivePath}>
      <activeRoute.Component activePath={activePath} />
    </Layout>
  );
}

function RequiredPasswordChange({ user, onChange, onLogout }) {
  const [form, setForm] = useState({ currentPassword: "", newPassword: "", confirmPassword: "" });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event) {
    event.preventDefault();
    setError("");
    if (form.newPassword !== form.confirmPassword) {
      setError("两次输入的新密码不一致");
      return;
    }
    setBusy(true);
    try {
      await onChange({ currentPassword: form.currentPassword, newPassword: form.newPassword });
    } catch (requestError) {
      setError(requestError.message || "修改密码失败");
    } finally {
      setBusy(false);
    }
  }
  return <main className="required-password-page"><form className="required-password-card" onSubmit={submit}><div className="brand-mark" aria-hidden="true"><span /></div><span>首次登录安全设置</span><h1>设置你的新密码</h1><p>{user.displayName || user.username}，临时密码仅用于首次登录。新密码至少 8 位，并同时包含字母和数字。</p><label><span>当前临时密码</span><input autoFocus className="text-input" required type="password" value={form.currentPassword} onChange={(event) => setForm({ ...form, currentPassword: event.target.value })} /></label><label><span>新密码</span><input className="text-input" minLength="8" required type="password" value={form.newPassword} onChange={(event) => setForm({ ...form, newPassword: event.target.value })} /></label><label><span>确认新密码</span><input className="text-input" minLength="8" required type="password" value={form.confirmPassword} onChange={(event) => setForm({ ...form, confirmPassword: event.target.value })} /></label>{error ? <div className="form-error">{error}</div> : null}<button className="primary-button" disabled={busy} type="submit">{busy ? "正在保存" : "保存并进入平台"}</button><button className="link-button" onClick={onLogout} type="button">退出登录</button></form></main>;
}
