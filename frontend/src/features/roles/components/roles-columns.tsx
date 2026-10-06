"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";

import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  RoleRowActionsMenu,
  type RoleRowActionHandlers,
} from "@/features/roles/components/role-row-actions";
import type { RoleSummary } from "@/features/roles/services/roles.service";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

export function useRolesColumns(handlers: RoleRowActionHandlers) {
  const { t, format } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<RoleSummary>({
          accessorKey: "name",
          labelKey: "roles.fields.name",
          // Name / slug are searched through the toolbar `q`.
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-2">
              <Link
                href={routes.platform.roles.detail(row.original.uuid)}
                className="font-medium hover:underline"
              >
                {row.original.name}
              </Link>
              {row.original.is_system ? (
                <Badge variant="secondary" className="text-[10px]">
                  {t("roles.labels.system")}
                </Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<RoleSummary>({
          accessorKey: "slug",
          labelKey: "roles.fields.slug",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.slug}
            </code>
          ),
        }),
        createColumn<RoleSummary>({
          id: "description",
          accessorKey: "description",
          labelKey: "roles.fields.description",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => row.original.description ?? "—",
        }),
        createColumn<RoleSummary>({
          id: "is_system",
          accessorFn: (row) => (row.is_system ? "true" : "false"),
          labelKey: "roles.fields.system",
          enableSorting: true,
          // `is_system=true|false` (TEC-365).
          filterVariant: "select",
          param: "is_system",
          filterOptions: [
            { value: "true", labelKey: "roles.labels.system", label: "system" },
            {
              value: "false",
              labelKey: "roles.labels.custom",
              label: "custom",
            },
          ],
          cell: ({ row }) =>
            row.original.is_system
              ? t("roles.labels.system")
              : t("roles.labels.custom"),
        }),
        createColumn<RoleSummary>({
          accessorKey: "created_at",
          labelKey: "roles.fields.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.created_at ? (
              <span className="text-sm tabular-nums">
                {format.dateTime(row.original.created_at)}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<RoleSummary>({
          id: "actions",
          labelKey: "roles.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <RoleRowActionsMenu role={row.original} handlers={handlers} />
          ),
        }),
      ] as ColumnDef<RoleSummary, unknown>[],
    [format, handlers, t],
  );
}
