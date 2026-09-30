"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { routes } from "@/config/routes";
import {
  ORGANIZATION_STATUS_TONE,
  ORGANIZATION_STATUS_VALUES,
} from "@/features/organizations/constants";
import {
  OrganizationRowActionsMenu,
  type OrganizationRowActionHandlers,
} from "@/features/organizations/components/organization-row-actions";
import type {
  Organization,
  OrganizationStatus,
} from "@/features/organizations/services/organizations.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function statusTone(status: string) {
  if (status in ORGANIZATION_STATUS_TONE) {
    return ORGANIZATION_STATUS_TONE[status as OrganizationStatus];
  }
  return "default" as const;
}

export type OrganizationsColumnsOptions = {
  handlers: OrganizationRowActionHandlers;
};

export function useOrganizationsColumns({
  handlers,
}: OrganizationsColumnsOptions) {
  const { t } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<Organization>({
          accessorKey: "name",
          labelKey: "organizations.columns.name",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<Organization>({
          accessorKey: "slug",
          labelKey: "organizations.columns.slug",
          enableSorting: true,
          filterVariant: "text",
          gridSecondary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.home(row.original.slug)}
              className="text-muted-foreground hover:text-foreground text-sm hover:underline"
              onClick={(event) => event.stopPropagation()}
            >
              /{row.original.slug}
            </Link>
          ),
        }),
        createColumn<Organization>({
          accessorKey: "city",
          labelKey: "organizations.columns.city",
          enableSorting: true,
          filterVariant: "text",
        }),
        createColumn<Organization>({
          accessorKey: "phone",
          labelKey: "organizations.columns.phone",
          enableSorting: false,
          filterVariant: "text",
        }),
        createColumn<Organization>({
          accessorKey: "plan_code",
          labelKey: "organizations.columns.plan_code",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => row.original.plan_code ?? "—",
        }),
        createColumn<Organization>({
          accessorKey: "status",
          labelKey: "organizations.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_STATUS_VALUES.map((value) => ({
            value,
            labelKey: `organizations.status.${value}`,
            label: value,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`organizations.status.${row.original.status}`)}
              tone={statusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Organization>({
          accessorKey: "access_ends_at",
          labelKey: "organizations.columns.access_ends_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.access_ends_at
              ? datetime(row.original.access_ends_at)
              : "—",
        }),
        createColumn<Organization>({
          accessorKey: "uuid",
          labelKey: "organizations.columns.uuid",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.uuid}
            </code>
          ),
        }),
        createColumn<Organization>({
          id: "actions",
          labelKey: "organizations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <OrganizationRowActionsMenu
              organization={row.original}
              handlers={handlers}
            />
          ),
        }),
      ] as ColumnDef<Organization, unknown>[],
    [handlers, t],
  );
}
