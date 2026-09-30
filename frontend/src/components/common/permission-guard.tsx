"use client";

import type { ReactNode } from "react";

import { usePermission } from "@/providers/permission-provider";

type PermissionGuardProps = {
  permission: string | string[];
  mode?: "all" | "any";
  fallback?: ReactNode;
  children: ReactNode;
};

/** Hide children unless the session has the required permission(s). */
export function PermissionGuard({
  permission,
  mode = "all",
  fallback = null,
  children,
}: PermissionGuardProps) {
  const { can, canAny } = usePermission();
  const allowed =
    mode === "any"
      ? canAny(Array.isArray(permission) ? permission : [permission])
      : can(permission);

  if (!allowed) return <>{fallback}</>;
  return <>{children}</>;
}
