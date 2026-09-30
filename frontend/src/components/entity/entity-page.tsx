"use client";

import type { ReactNode } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { PageHeader } from "@/components/layout";
import type { BreadcrumbItem } from "@/components/layout/breadcrumb";

type EntityPageProps = {
  title: string;
  description?: string;
  breadcrumbs?: BreadcrumbItem[];
  actions?: ReactNode;
  /** When set, page content is gated behind PermissionGuard */
  permission?: string | string[];
  permissionMode?: "all" | "any";
  forbiddenFallback?: ReactNode;
  children: ReactNode;
};

/**
 * Standard entity list/detail page shell: header + optional permission gate.
 */
export function EntityPage({
  title,
  description,
  breadcrumbs,
  actions,
  permission,
  permissionMode = "all",
  forbiddenFallback = null,
  children,
}: EntityPageProps) {
  const body = (
    <>
      <PageHeader
        title={title}
        description={description}
        breadcrumbs={breadcrumbs}
        actions={actions}
      />
      {children}
    </>
  );

  if (!permission) return body;

  return (
    <PermissionGuard
      permission={permission}
      mode={permissionMode}
      fallback={forbiddenFallback}
    >
      {body}
    </PermissionGuard>
  );
}
