"use client";

import type { ReactNode } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";

type CanProps = {
  permission: string | string[];
  mode?: "all" | "any";
  fallback?: ReactNode;
  children: ReactNode;
};

/**
 * Permission gate — thin alias over PermissionGuard (single implementation).
 */
export function Can({
  permission,
  mode = "all",
  fallback = null,
  children,
}: CanProps) {
  return (
    <PermissionGuard permission={permission} mode={mode} fallback={fallback}>
      {children}
    </PermissionGuard>
  );
}

/** Alias for Can — permission foundation naming. */
export const HasPermission = Can;
