import { useMemo, useState } from "react";
import { Layout } from "./components/Layout.js";
import { LoginPage } from "./pages/LoginPage.js";
import { routes } from "./routes/index.js";
import { useAuth } from "./hooks/useAuth.js";

export default function App() {
  const { user, bootstrapping } = useAuth();
  const [activePath, setActivePath] = useState(["首页", "项目概览"]);

  const activeRoute = useMemo(() => {
    return routes.find((route) => route.match(activePath)) || routes[0];
  }, [activePath]);

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
