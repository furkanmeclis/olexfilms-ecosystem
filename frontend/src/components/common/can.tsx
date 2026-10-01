"use client";

import type { ReactNode } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import type { PermissionScope } from "@/config/permissions";

type CanProps = {
  permission: string | string[];
  mode?: "all" | "any";
  scope?: PermissionScope;
  fallback?: ReactNode;
  children: ReactNode;
};

/**
 * Permission gate — thin alias over PermissionGuard (single implementation).
 */
export function Can({
  permission,
  mode = "all",
  scope,
  fallback = null,
  children,
}: CanProps) {
  return (
    <PermissionGuard
      permission={permission}
      mode={mode}
      scope={scope}
      fallback={fallback}
    >
      {children}
    </PermissionGuard>
  );
}

/** Alias for Can — permission foundation naming. */
export const HasPermission = Can;
