"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { createColumn } from "@/components/tables";
import { routes } from "@/config/routes";
import {
  USER_STATUS_TONE,
  USER_STATUS_VALUES,
} from "@/features/users/constants";
import { UserAuthMethodsIcons } from "@/features/users/components/user-auth-methods";
import {
  UserRowActionsMenu,
  type UserRowActionHandlers,
} from "@/features/users/components/user-row-actions";
import { userFullName } from "@/features/users/lib/user-display";
import { roleDisplayName } from "@/features/roles/lib/role-display";
import type {
  PublicUser,
  UserStatus,
} from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

function statusTone(status: string) {
  if (status in USER_STATUS_TONE) {
    return USER_STATUS_TONE[status as UserStatus];
  }
  return "default" as const;
}

export type UsersColumnsOptions = {
  handlers: UserRowActionHandlers;
  /** Role filter options (value = role slug, the CSV `role` query param). */
  roleOptions?: { value: string; label: string }[];
  currentUserUuid?: string;
  canImpersonateSuperAdmin?: boolean;
  isImpersonating?: boolean;
};

export function useUsersColumns({
  handlers,
  roleOptions,
  currentUserUuid,
  canImpersonateSuperAdmin,
  isImpersonating,
}: UsersColumnsOptions) {
  const { t, format } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<PublicUser>({
          id: "name",
          accessorFn: (row) => userFullName(row),
          labelKey: "users.columns.name",
          // Name / email are searched through the toolbar `q`.
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{userFullName(row.original)}</span>
          ),
        }),
        createColumn<PublicUser>({
          accessorKey: "email",
          labelKey: "users.columns.email",
          enableSorting: true,
          gridSecondary: true,
        }),
        createColumn<PublicUser>({
          accessorKey: "status",
          labelKey: "users.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          param: "status",
          filterOptions: USER_STATUS_VALUES.map((value) => ({
            value,
            labelKey: `users.status.${value}`,
            label: value,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`users.status.${row.original.status}`)}
              tone={statusTone(row.original.status)}
            />
          ),
        }),
        createColumn<PublicUser>({
          id: "roles",
          accessorFn: (row) =>
            (row.roles ?? [])
              .map((role) => roleDisplayName(role, t))
              .join(", "),
          labelKey: "users.columns.roles",
          enableSorting: false,
          // Multi-select role filter → `role=a,b` (CSV slugs, TEC-365).
          filterVariant: "faceted",
          filterOptions: roleOptions ?? [],
          // Without the role catalog (no roles.read) there is nothing to
          // pick; page-local facet values would not be slugs.
          enableColumnFilter: Boolean(roleOptions?.length),
          param: "role",
          cell: ({ row }) => {
            const roles = row.original.roles ?? [];
            if (roles.length === 0) {
              return (
                <span className="text-muted-foreground text-sm">
                  {t("users.detail.roles_empty")}
                </span>
              );
            }
            return (
              <div className="flex flex-wrap gap-1">
                {roles.map((role) => (
                  <Badge
                    key={role.uuid}
                    variant="secondary"
                    className="max-w-40 truncate font-normal"
                    title={role.slug}
                  >
                    <Link
                      href={routes.platform.roles.detail(role.uuid)}
                      className="hover:underline"
                      onClick={(event) => event.stopPropagation()}
                    >
                      {roleDisplayName(role, t)}
                    </Link>
                  </Badge>
                ))}
              </div>
            );
          },
        }),
        createColumn<PublicUser>({
          id: "auth_methods",
          accessorFn: (row) =>
            (row.auth_methods ?? [])
              .map((method) => method.provider ?? method.kind)
              .join(", "),
          labelKey: "users.columns.auth_methods",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <UserAuthMethodsIcons methods={row.original.auth_methods ?? []} />
          ),
        }),
        createColumn<PublicUser>({
          accessorKey: "uuid",
          labelKey: "users.columns.uuid",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.uuid}
            </code>
          ),
        }),
        createColumn<PublicUser>({
          accessorKey: "created_at",
          labelKey: "users.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) =>
            row.original.created_at ? (
              <span className="text-sm tabular-nums">
                {format.dateTime(row.original.created_at)}
              </span>
            ) : (
              <span className="text-muted-foreground text-sm">—</span>
            ),
        }),
        createColumn<PublicUser>({
          id: "actions",
          labelKey: "users.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <UserRowActionsMenu
              user={row.original}
              handlers={handlers}
              currentUserUuid={currentUserUuid}
              canImpersonateSuperAdmin={canImpersonateSuperAdmin}
              isImpersonating={isImpersonating}
            />
          ),
        }),
      ] as ColumnDef<PublicUser, unknown>[],
    [
      canImpersonateSuperAdmin,
      currentUserUuid,
      format,
      handlers,
      isImpersonating,
      roleOptions,
      t,
    ],
  );
}
