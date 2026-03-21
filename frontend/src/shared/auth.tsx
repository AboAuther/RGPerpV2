import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import type { PropsWithChildren } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import type { AuthenticatedSession, User } from './domain';
import { setApiAccessToken } from './api';

interface AuthContextValue {
  session: AuthenticatedSession | null;
  isAuthenticated: boolean;
  signIn: (session: AuthenticatedSession) => void;
  signOut: () => void;
}

const sessionStorageKey = 'rgperp.session';

const AuthContext = createContext<AuthContextValue | null>(null);

function isSessionExpired(session: AuthenticatedSession): boolean {
  if (!session.expiresAt) {
    return false;
  }

  const expiresAt = Date.parse(session.expiresAt);
  if (Number.isNaN(expiresAt)) {
    return true;
  }

  return expiresAt <= Date.now();
}

function readPersistedSession(): AuthenticatedSession | null {
  const raw = window.sessionStorage.getItem(sessionStorageKey);
  if (!raw) {
    return null;
  }

  try {
    const parsed = JSON.parse(raw) as AuthenticatedSession;
    if (!parsed.accessToken || !parsed.user) {
      window.sessionStorage.removeItem(sessionStorageKey);
      return null;
    }
    if (isSessionExpired(parsed)) {
      window.sessionStorage.removeItem(sessionStorageKey);
      return null;
    }
    return parsed;
  } catch {
    window.sessionStorage.removeItem(sessionStorageKey);
    return null;
  }
}

const adminRoles = new Set(['admin', 'super_admin', 'risk_admin', 'ops_admin']);
const adminCapabilities = new Set(['admin', 'admin:*', 'admin.dashboard', 'admin.withdrawals', 'admin.configs', 'admin.liquidations']);

export function hasAdminAccess(user: User | null | undefined): boolean {
  if (!user) {
    return false;
  }

  if (user.is_admin) {
    return true;
  }

  const normalizedRole = user.role?.trim().toLowerCase();
  if (normalizedRole && adminRoles.has(normalizedRole)) {
    return true;
  }

  return (user.capabilities || []).some((capability) => adminCapabilities.has(capability.trim().toLowerCase()));
}

export function AuthProvider({ children }: PropsWithChildren) {
  const [session, setSession] = useState<AuthenticatedSession | null>(() => {
    if (typeof window === 'undefined') {
      return null;
    }
    const restored = readPersistedSession();
    setApiAccessToken(restored?.accessToken);
    return restored;
  });

  useEffect(() => {
    setApiAccessToken(session?.accessToken);
  }, [session]);

  const value = useMemo<AuthContextValue>(
    () => ({
      session,
      isAuthenticated: !!session,
      signIn(nextSession) {
        setApiAccessToken(nextSession.accessToken);
        setSession(nextSession);
        window.sessionStorage.setItem(sessionStorageKey, JSON.stringify(nextSession));
      },
      signOut() {
        setApiAccessToken(undefined);
        setSession(null);
        window.sessionStorage.removeItem(sessionStorageKey);
      },
    }),
    [session],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
}

export function ProtectedOutlet() {
  const { isAuthenticated } = useAuth();
  const location = useLocation();

  if (!isAuthenticated) {
    return <Navigate replace to="/login" state={{ from: location.pathname }} />;
  }

  return <Outlet />;
}

export function AdminOutlet() {
  const { isAuthenticated, session } = useAuth();
  const location = useLocation();

  if (!isAuthenticated) {
    return <Navigate replace to="/login" state={{ from: location.pathname }} />;
  }

  if (!hasAdminAccess(session?.user)) {
    return <Navigate replace to="/portfolio" />;
  }

  return <Outlet />;
}
