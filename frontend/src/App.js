import { useEffect, useMemo, useState } from "react";
import { Layout } from "./components/Layout.js";
import { LoginPage } from "./pages/LoginPage.js";
import { routes } from "./routes/index.js";
import { useAuth } from "./hooks/useAuth.js";
import { cleanupRouteState, getDefaultPath, pathFromHash, persistRouteState, readRouteState, readScrollState } from "./utils/routeState.js";

export default function App() {
  const { user, bootstrapping } = useAuth();
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

  return (
    <Layout activePath={activePath} onNavigate={setActivePath}>
      <activeRoute.Component activePath={activePath} />
    </Layout>
  );
}
