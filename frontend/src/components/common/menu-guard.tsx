"use client";

import type { ReactNode } from "react";

import { usePermission } from "@/providers/permission-provider";

type MenuGuardProps = {
  permission?: string | string[];
  role?: string | string[];
  children: ReactNode;
};

/**
 * Soft menu visibility guard. Does not replace API authorization.
 */
export function MenuGuard({ permission, role, children }: MenuGuardProps) {
  const { can, hasRole } = usePermission();

  if (permission && !can(permission)) return null;
  if (role && !hasRole(role)) return null;
  return <>{children}</>;
}
