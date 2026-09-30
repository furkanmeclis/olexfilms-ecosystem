"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { SessionProvider, signOut, useSession } from "next-auth/react";

import { setAuthFailureHandler } from "@/lib/api";
import { useAuthStore, type AuthUser } from "@/lib/auth/session-store";
import { mapMeToAuthUser } from "@/lib/auth/types";
import { setClientSessionFlag } from "@/lib/routing/guards";
import { authService } from "@/services/auth.service";

type AuthContextValue = {
  user: AuthUser | null;
  isAuthenticated: boolean;
  bootstrapped: boolean;
  setUser: (user: AuthUser | null) => void;
  markAuthenticated: () => void;
  logout: () => Promise<void>;
  refreshSession: () => Promise<boolean>;
  hydrateProfile: () => Promise<AuthUser | null>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

function AuthContextBridge({ children }: { children: ReactNode }) {
  const { data: session, status, update } = useSession();
  const user = useAuthStore((s) => s.user);
  const bootstrapped = useAuthStore((s) => s.bootstrapped);
  const setUser = useAuthStore((s) => s.setUser);
  const clear = useAuthStore((s) => s.clear);
  const setBootstrapped = useAuthStore((s) => s.setBootstrapped);
  // Optimistic: if a cached user exists in sessionStorage, start as authenticated
  // so the tenant layout renders without a loading screen while session verifies.
  const [sessionAuthenticated, setSessionAuthenticated] = useState(() =>
    Boolean(useAuthStore.getState().user),
  );

  const hydrateProfile = useCallback(async () => {
    try {
      const me = await authService.getMe();
      const mapped = mapMeToAuthUser(me);
      setUser(mapped);
      return mapped;
    } catch {
      return null;
    }
  }, [setUser]);

  const applySession = useCallback(
    (authenticated: boolean) => {
      setSessionAuthenticated(authenticated);
      setClientSessionFlag(authenticated);
      if (!authenticated) clear();
    },
    [clear],
  );

  const refreshSession = useCallback(async () => {
    try {
      const result = await authService.refresh();
      if (result.authenticated) {
        await update();
        applySession(true);
        await hydrateProfile();
        return true;
      }
      applySession(false);
      return false;
    } catch {
      applySession(false);
      return false;
    }
  }, [applySession, hydrateProfile, update]);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      if (status === "loading") return;

      const authenticated =
        status === "authenticated" &&
        !(session as { error?: string } | null)?.error;
      applySession(authenticated);
      if (authenticated) {
        await hydrateProfile();
      }
      if (!cancelled) setBootstrapped(true);
    })();

    setAuthFailureHandler(async () => {
      applySession(false);
      await signOut({ redirect: false });
    });

    return () => {
      cancelled = true;
      setAuthFailureHandler(null);
    };
  }, [applySession, hydrateProfile, setBootstrapped, session, status]);

  const markAuthenticated = useCallback(() => {
    applySession(true);
  }, [applySession]);

  const logout = useCallback(async () => {
    await authService.logout();
    applySession(false);
    await signOut({ redirect: false });
  }, [applySession]);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      isAuthenticated: sessionAuthenticated || Boolean(user),
      bootstrapped,
      setUser,
      markAuthenticated,
      logout,
      refreshSession,
      hydrateProfile,
    }),
    [
      user,
      sessionAuthenticated,
      bootstrapped,
      setUser,
      markAuthenticated,
      logout,
      refreshSession,
      hydrateProfile,
    ],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  return (
    <SessionProvider>
      <AuthContextBridge>{children}</AuthContextBridge>
    </SessionProvider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
