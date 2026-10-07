"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Gauge } from "lucide-react";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityRowActions } from "@/components/entity/entity-row-actions";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { QuotaBar } from "@/features/ai-admin/components/quota-bar";
import { AI_ORG_TYPES, isUnlimitedQuota } from "@/features/ai-admin/lib/quota";
import type { AIOrgQuota } from "@/features/ai-admin/services/ai-admin.service";
import { useLocale } from "@/providers/locale-provider";

export type OrgQuotaRowHandlers = {
  onEditQuota: (row: AIOrgQuota) => void;
};

/**
 * Quota table columns (GET /v1/platform/ai/orgs): sort `name` | `quota` |
 * `usage`, `org_type` faceted filter; `q` is the toolbar search.
 */
export function useOrgQuotasColumns({
  handlers,
}: {
  handlers: OrgQuotaRowHandlers;
}) {
  const { t, format } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<AIOrgQuota>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "ai_admin.columns.organization",
          enableSorting: true,
          sortParam: "name",
          enableColumnFilter: false,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.organization.name}
            </span>
          ),
        }),
        createColumn<AIOrgQuota>({
          id: "org_type",
          accessorFn: (row) => row.organization.type,
          labelKey: "ai_admin.columns.org_type",
          enableSorting: false,
          filterVariant: "faceted",
          param: "org_type",
          filterOptions: AI_ORG_TYPES.map((value) => ({
            value,
            labelKey: `ai_admin.org_type.${value}`,
            label: value,
          })),
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge variant="outline" className="font-normal">
              {t(`ai_admin.org_type.${row.original.organization.type}`)}
            </Badge>
          ),
        }),
        createColumn<AIOrgQuota>({
          accessorKey: "quota",
          labelKey: "ai_admin.columns.quota",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-sm tabular-nums">
                {isUnlimitedQuota(row.original.quota)
                  ? t("ai_admin.quota.unlimited")
                  : format.number(row.original.quota)}
              </span>
              {row.original.quota_override == null ? (
                <Badge variant="secondary" className="font-normal">
                  {t("ai_admin.quota.default_badge")}
                </Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<AIOrgQuota>({
          accessorKey: "used",
          labelKey: "ai_admin.columns.used",
          enableSorting: true,
          sortParam: "usage",
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {format.number(row.original.used)}
            </span>
          ),
        }),
        createColumn<AIOrgQuota>({
          accessorKey: "percent",
          labelKey: "ai_admin.columns.percent",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => <QuotaBar percent={row.original.percent} />,
        }),
        createColumn<AIOrgQuota>({
          accessorKey: "request_count",
          labelKey: "ai_admin.columns.request_count",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {format.number(row.original.request_count)}
            </span>
          ),
        }),
        createColumn<AIOrgQuota>({
          accessorKey: "system_used",
          labelKey: "ai_admin.columns.system_used",
          enableSorting: false,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {format.number(row.original.system_used)}
            </span>
          ),
        }),
        createColumn<AIOrgQuota>({
          accessorKey: "enabled",
          labelKey: "ai_admin.columns.enabled",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(
                row.original.enabled
                  ? "ai_admin.status.enabled"
                  : "ai_admin.status.disabled",
              )}
              tone={row.original.enabled ? "success" : "default"}
            />
          ),
        }),
        createColumn<AIOrgQuota>({
          id: "actions",
          labelKey: "ai_admin.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit-quota",
                  label: t("ai_admin.actions.edit_quota"),
                  icon: Gauge,
                  permission: permissions.aiAdmin.settingsManage,
                  onSelect: () => handlers.onEditQuota(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AIOrgQuota, unknown>[],
    [format, handlers, t],
  );
}
