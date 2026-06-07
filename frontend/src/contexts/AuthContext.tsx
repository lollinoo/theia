/**
 * Provides auth context context state for the React application.
 * Centralizes shared lifecycle and persistence behavior behind a stable provider contract.
 */
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';
import {
  type AuthSession,
  type AuthUser,
  type ChangePasswordPayload,
  changePassword as changePasswordRequest,
  fetchCurrentUser,
  type LoginPayload,
  loginUser,
  logoutUser,
} from '../api/client';

type AuthStatus = 'checking' | 'authenticated' | 'unauthenticated';

interface AuthContextValue {
  status: AuthStatus;
  user: AuthUser | null;
  error: string | null;
  refresh: () => Promise<AuthSession>;
  login: (payload: LoginPayload) => Promise<AuthSession>;
  logout: () => Promise<void>;
  changePassword: (payload: ChangePasswordPayload) => Promise<AuthSession>;
  hasPermission: (permission: string) => boolean;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function authErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

/** Renders the AuthProvider component within the shared React context. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('checking');
  const [user, setUser] = useState<AuthUser | null>(null);
  const [error, setError] = useState<string | null>(null);

  const applySession = useCallback((session: AuthSession): AuthSession => {
    if (session.authenticated && session.user) {
      setUser(session.user);
      setStatus('authenticated');
      setError(null);
      return session;
    }

    setUser(null);
    setStatus('unauthenticated');
    return session;
  }, []);

  const refresh = useCallback(async () => {
    setStatus((current) => (current === 'authenticated' ? current : 'checking'));
    try {
      const session = await fetchCurrentUser();
      return applySession(session);
    } catch (refreshError) {
      setUser(null);
      setStatus('unauthenticated');
      setError(authErrorMessage(refreshError, 'Unable to check session'));
      throw refreshError;
    }
  }, [applySession]);

  useEffect(() => {
    let cancelled = false;
    fetchCurrentUser()
      .then((session) => {
        if (!cancelled) {
          applySession(session);
        }
      })
      .catch((sessionError) => {
        if (!cancelled) {
          setUser(null);
          setStatus('unauthenticated');
          setError(authErrorMessage(sessionError, 'Unable to check session'));
        }
      });

    return () => {
      cancelled = true;
    };
  }, [applySession]);

  const login = useCallback(
    async (payload: LoginPayload) => {
      const session = await loginUser(payload);
      return applySession(session);
    },
    [applySession],
  );

  const logout = useCallback(async () => {
    setError(null);
    try {
      const session = await logoutUser();
      applySession(session);
    } catch (logoutError) {
      setError('Unable to log out. Check your connection and try again.');
      throw logoutError;
    }
  }, [applySession]);

  const changePassword = useCallback(
    async (payload: ChangePasswordPayload) => {
      const session = await changePasswordRequest(payload);
      return applySession(session);
    },
    [applySession],
  );

  const hasPermission = useCallback(
    (permission: string) => user?.permissions.includes(permission) === true,
    [user],
  );

  const value = useMemo(
    () => ({
      status,
      user,
      error,
      refresh,
      login,
      logout,
      changePassword,
      hasPermission,
    }),
    [changePassword, error, hasPermission, login, logout, refresh, status, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/** Coordinates auth behavior for the shared React context. */
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    return {
      status: 'unauthenticated',
      user: null,
      error: null,
      refresh: async () => ({ authenticated: false }),
      login: async () => ({ authenticated: false }),
      logout: async () => {},
      changePassword: async () => ({ authenticated: false }),
      hasPermission: () => false,
    };
  }
  return context;
}
