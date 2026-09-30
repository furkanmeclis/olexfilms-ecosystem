"use client";

import type { ReactNode } from "react";

import { ExportMenu } from "@/features/io/components/export-menu";
import { ImportButton } from "@/features/io/components/import-button";
import type {
  IoResource,
  ResourceCapabilities,
  ExportJobScope,
} from "@/features/io/types";
import { permissions } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";

type ResourceIOToolbarProps = {
  resource: IoResource;
  query?: Record<string, string | undefined>;
  capabilities?: ResourceCapabilities;
  onImportComplete?: () => void;
  jobsHref?: string;
  importJobsHref?: string;
  scope?: ExportJobScope;
  /** Override default EXPORT_PATHS entry (e.g. cari statement per account). */
  exportPath?: string;
};

const EXPORT_PERM: Partial<Record<IoResource, string>> = {
  "platform.users": permissions.users.export,
  "platform.roles": permissions.roles.export,
  "platform.notifications": permissions.notifications.export,
  "platform.activity": permissions.activity.read,
};

const IMPORT_PERM: Partial<Record<IoResource, string>> = {
  "platform.users": permissions.users.import,
  "platform.roles": permissions.roles.import,
};

export function ResourceIOToolbar({
  resource,
  query,
  capabilities,
  onImportComplete,
  jobsHref,
  importJobsHref,
  scope = "platform",
  exportPath,
}: ResourceIOToolbarProps) {
  const { can } = usePermission();
  const nodes: ReactNode[] = [];

  const exportPerm = EXPORT_PERM[resource];
  if (capabilities?.export && exportPerm && can(exportPerm)) {
    nodes.push(
      <ExportMenu
        key="export"
        resource={resource}
        query={query}
        jobsHref={jobsHref}
        exportPath={exportPath}
      />,
    );
  }

  const importPerm = IMPORT_PERM[resource];
  if (capabilities?.import && importPerm && can(importPerm)) {
    nodes.push(
      <ImportButton
        key="import"
        resource={resource}
        onComplete={onImportComplete}
        jobsHref={importJobsHref}
        scope={scope}
      />,
    );
  }

  if (!nodes.length) return null;
  return <>{nodes}</>;
}
