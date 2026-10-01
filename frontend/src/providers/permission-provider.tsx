"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";

import { scopeCovers, type PermissionScope } from "@/config/permissions";
import { useAuthStore } from "@/lib/auth/session-store";

type PermissionContextValue = {
  permissions: string[];
  grants: Record<string, PermissionScope>;
  roles: string[];
  can: (permission: string | string[]) => boolean;
  /** Scope held for a permission in the active organization (TEC-85). */
  scopeFor: (permission: string) => PermissionScope | undefined;
  /** Permission held with a scope covering `need` (e.g. subtree ⊇ managed). */
  canScope: (permission: string, need: PermissionScope) => boolean;
  canAny: (permissions: string[]) => boolean;
  canAll: (permissions: string[]) => boolean;
  hasPermission: (permission: string | string[]) => boolean;
  hasRole: (role: string | string[]) => boolean;
};

const PermissionContext = createContext<PermissionContextValue | null>(null);

const EMPTY_PERMISSIONS: string[] = [];
const EMPTY_ROLES: string[] = [];
const EMPTY_GRANTS: Record<string, PermissionScope> = {};

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
  const grants = useAuthStore((s) => s.user?.grants ?? EMPTY_GRANTS);
  const isSuperAdmin = useAuthStore((s) => Boolean(s.user?.isSuperAdmin));

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

    const scopeFor = (permission: string): PermissionScope | undefined => {
      const scope = grants[permission];
      if (scope) return scope;
      if (isSuperAdmin) return "all";
      return permissionSet.has(permission) ? "all" : undefined;
    };

    return {
      permissions,
      grants,
      roles,
      can,
      scopeFor,
      canScope: (permission, need) => {
        const scope = scopeFor(permission);
        return scope ? scopeCovers(scope, need) : false;
      },
      canAny: (list) => list.some((p) => permissionSet.has(p)),
      canAll: (list) => list.every((p) => permissionSet.has(p)),
      hasPermission: can,
      hasRole,
    };
  }, [permissions, roles, grants, isSuperAdmin]);

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
