"use client";

import type { ReactNode } from "react";

import { usePermission } from "@/providers/permission-provider";

type HasRoleProps = {
  role: string | string[];
  fallback?: ReactNode;
  children: ReactNode;
};

export function HasRole({ role, fallback = null, children }: HasRoleProps) {
  const { hasRole } = usePermission();
  if (!hasRole(role)) return <>{fallback}</>;
  return <>{children}</>;
}
