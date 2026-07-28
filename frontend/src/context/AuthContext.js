import { createContext, useCallback, useEffect, useMemo, useState } from "react";
import { changePassword as changePasswordRequest, currentUser, login as loginRequest, logout as clearSession } from "../services/authService.js";
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
  const changePassword = useCallback(async (payload) => {
    const nextUser = await changePasswordRequest(payload);
    setUser(nextUser);
  }, []);

  const value = useMemo(() => ({ user, bootstrapping, login, logout, changePassword }), [user, bootstrapping, login, logout, changePassword]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
