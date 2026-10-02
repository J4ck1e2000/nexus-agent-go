import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import type {
  LoginPayload,
  NexusResult,
  NexusUser,
  RegisterPayload,
} from '../../electron/types/ipc';

type AuthStatus = 'loading' | 'authenticated' | 'guest';

interface AuthContextValue {
  user: NexusUser | null;
  status: AuthStatus;
  login: (payload: LoginPayload) => Promise<NexusResult<NexusUser>>;
  register: (payload: RegisterPayload) => Promise<NexusResult<NexusUser>>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<NexusUser | null>(null);
  const [status, setStatus] = useState<AuthStatus>('loading');

  // Bootstrap: restore the session from the main-process token store.
  useEffect(() => {
    let disposed = false;
    void window.nexus.auth.me().then((result) => {
      if (disposed) return;
      if (result.ok) {
        setUser(result.data);
        setStatus('authenticated');
      } else {
        setUser(null);
        setStatus('guest');
      }
    });
    const unsubscribe = window.nexus.auth.onAuthExpired(() => {
      setUser(null);
      setStatus('guest');
    });
    return () => {
      disposed = true;
      unsubscribe();
    };
  }, []);

  const login = useCallback(async (payload: LoginPayload) => {
    const result = await window.nexus.auth.login(payload);
    if (result.ok) {
      setUser(result.data);
      setStatus('authenticated');
    }
    return result;
  }, []);

  const register = useCallback(async (payload: RegisterPayload) => {
    // Registration does not sign the user in; the caller switches to login mode.
    return window.nexus.auth.register(payload);
  }, []);

  const logout = useCallback(async () => {
    await window.nexus.auth.logout();
    setUser(null);
    setStatus('guest');
  }, []);

  const value = useMemo(
    () => ({ user, status, login, register, logout }),
    [user, status, login, register, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuthContext(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuthContext must be used inside AuthProvider');
  }
  return ctx;
}
