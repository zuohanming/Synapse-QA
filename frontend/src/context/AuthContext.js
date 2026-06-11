import { createContext, useCallback, useEffect, useMemo, useState } from "react";
import { currentUser, login as loginRequest, logout as clearSession } from "../services/authService.js";
import { getToken } from "../services/httpClient.js";

export const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [bootstrapping, setBootstrapping] = useState(true);

  useEffect(() => {
    let alive = true;
    async function bootstrap() {
      if (!getToken()) {
        setBootstrapping(false);
        return;
      }
      try {
        const me = await currentUser();
        if (alive) setUser(me);
      } catch {
        clearSession();
      } finally {
        if (alive) setBootstrapping(false);
      }
    }
    bootstrap();
    return () => {
      alive = false;
    };
  }, []);

  const login = useCallback(async (credentials) => {
    const nextUser = await loginRequest(credentials);
    setUser(nextUser);
  }, []);

  const logout = useCallback(() => {
    clearSession();
    setUser(null);
  }, []);

  const value = useMemo(() => ({ user, bootstrapping, login, logout }), [user, bootstrapping, login, logout]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
