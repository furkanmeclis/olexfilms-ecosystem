import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import type { AuthUser } from "@/lib/auth/types";

type AuthState = {
  user: AuthUser | null;
  bootstrapped: boolean;
  setUser: (user: AuthUser | null) => void;
  setBootstrapped: (value: boolean) => void;
  clear: () => void;
};

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      bootstrapped: false,
      setUser: (user) => set({ user }),
      setBootstrapped: (bootstrapped) => set({ bootstrapped }),
      clear: () => set({ user: null, bootstrapped: false }),
    }),
    {
      name: "olexfilms-auth-v1",
      storage: createJSONStorage(() => {
        // sessionStorage is unavailable during SSR; fall back to a no-op store.
        if (typeof window === "undefined") {
          return {
            getItem: () => null,
            setItem: () => {},
            removeItem: () => {},
          };
        }
        return sessionStorage;
      }),
      // Only persist the user profile — bootstrapped is always derived at runtime.
      partialize: (state) => ({ user: state.user }),
    },
  ),
);

export type { AuthUser };
