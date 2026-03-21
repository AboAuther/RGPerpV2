import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import type { PropsWithChildren } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import type { AuthenticatedSession } from './domain';
import { setApiAccessToken } from './api';
import { appConfig } from './env';

interface AuthContextValue {
  session: AuthenticatedSession | null;
  isAuthenticated: boolean;
  signIn: (session: AuthenticatedSession) => void;
  signOut: () => void;
}

const mockSessionStorageKey = 'rgperp.mock.session';

const AuthContext = createContext<AuthContextValue | null>(null);

function readMockSession(): AuthenticatedSession | null {
  const raw = window.sessionStorage.getItem(mockSessionStorageKey);
  if (!raw) {
    return null;
  }

  try {
    const parsed = JSON.parse(raw) as AuthenticatedSession;
    if (parsed.provider !== 'mock') {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export function AuthProvider({ children }: PropsWithChildren) {
  const [session, setSession] = useState<AuthenticatedSession | null>(null);

  useEffect(() => {
    const restored = readMockSession();
    if (restored) {
      setSession(restored);
    }
  }, []);

  useEffect(() => {
    setApiAccessToken(session?.accessToken);
  }, [session]);

  const value = useMemo<AuthContextValue>(
    () => ({
      session,
      isAuthenticated: !!session,
      signIn(nextSession) {
        setSession(nextSession);
        if (nextSession.provider === 'mock') {
          window.sessionStorage.setItem(mockSessionStorageKey, JSON.stringify(nextSession));
        } else {
          window.sessionStorage.removeItem(mockSessionStorageKey);
        }
      },
      signOut() {
        setSession(null);
        window.sessionStorage.removeItem(mockSessionStorageKey);
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

  if (appConfig.disableRouteGuard) {
    return <Outlet />;
  }

  if (!isAuthenticated) {
    return <Navigate replace to="/login" state={{ from: location.pathname }} />;
  }

  return <Outlet />;
}
