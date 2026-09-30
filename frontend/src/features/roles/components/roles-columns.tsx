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
  const { t } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<RoleSummary>({
          accessorKey: "name",
          labelKey: "roles.fields.name",
          enableSorting: false,
          filterVariant: "text",
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
          enableSorting: false,
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
    [handlers, t],
  );
}
