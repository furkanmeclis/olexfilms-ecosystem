"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";

import { useAuthStore } from "@/lib/auth/session-store";

type PermissionContextValue = {
  permissions: string[];
  roles: string[];
  can: (permission: string | string[]) => boolean;
  canAny: (permissions: string[]) => boolean;
  canAll: (permissions: string[]) => boolean;
  hasPermission: (permission: string | string[]) => boolean;
  hasRole: (role: string | string[]) => boolean;
};

const PermissionContext = createContext<PermissionContextValue | null>(null);

const EMPTY_PERMISSIONS: string[] = [];
const EMPTY_ROLES: string[] = [];

/** ADR-004: `*.manage` implies granular view/create/update/delete (and permissions.view). */
const MANAGE_IMPLIES: Record<string, string[]> = {
  "roles.manage": [
    "roles.view",
    "roles.create",
    "roles.update",
    "roles.delete",
  ],
  "permissions.manage": ["permissions.view"],
};

function expandPermissionSet(permissions: string[]) {
  const set = new Set(permissions);
  for (const [manage, implied] of Object.entries(MANAGE_IMPLIES)) {
    if (set.has(manage)) {
      for (const slug of implied) set.add(slug);
    }
  }
  return set;
}

export function PermissionProvider({ children }: { children: ReactNode }) {
  const permissions = useAuthStore(
    (s) => s.user?.permissions ?? EMPTY_PERMISSIONS,
  );
  const roles = useAuthStore((s) => s.user?.roles ?? EMPTY_ROLES);

  const value = useMemo<PermissionContextValue>(() => {
    const permissionSet = expandPermissionSet(permissions);
    const roleSet = new Set(roles);

    const can = (permission: string | string[]) => {
      if (Array.isArray(permission)) {
        return permission.every((p) => permissionSet.has(p));
      }
      return permissionSet.has(permission);
    };

    const hasRole = (role: string | string[]) => {
      if (Array.isArray(role)) return role.some((r) => roleSet.has(r));
      return roleSet.has(role);
    };

    return {
      permissions,
      roles,
      can,
      canAny: (list) => list.some((p) => permissionSet.has(p)),
      canAll: (list) => list.every((p) => permissionSet.has(p)),
      hasPermission: can,
      hasRole,
    };
  }, [permissions, roles]);

  return (
    <PermissionContext.Provider value={value}>
      {children}
    </PermissionContext.Provider>
  );
}

export function usePermission() {
  const ctx = useContext(PermissionContext);
  if (!ctx) {
    throw new Error("usePermission must be used within PermissionProvider");
  }
  return ctx;
}
