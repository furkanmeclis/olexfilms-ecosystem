"use client";

import type { ReactNode } from "react";

import type { PermissionScope } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";

type PermissionGuardProps = {
  permission: string | string[];
  mode?: "all" | "any";
  /** Minimum scope each permission must be held with (e.g. "subtree"). */
  scope?: PermissionScope;
  fallback?: ReactNode;
  children: ReactNode;
};

/** Hide children unless the session has the required permission(s). */
export function PermissionGuard({
  permission,
  mode = "all",
  scope,
  fallback = null,
  children,
}: PermissionGuardProps) {
  const { can, canAny, canScope } = usePermission();
  const list = Array.isArray(permission) ? permission : [permission];
  const allowed = scope
    ? mode === "any"
      ? list.some((p) => canScope(p, scope))
      : list.every((p) => canScope(p, scope))
    : mode === "any"
      ? canAny(list)
      : can(permission);

  if (!allowed) return <>{fallback}</>;
  return <>{children}</>;
}
