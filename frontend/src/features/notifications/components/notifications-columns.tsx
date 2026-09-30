"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import {
  NOTIFICATION_STATUS_VALUES,
  priorityTone,
  statusTone,
} from "@/features/notifications/constants";
import {
  NotificationRowActionsMenu,
  type NotificationRowActionHandlers,
} from "@/features/notifications/components/notification-row-actions";
import type { Notification } from "@/features/notifications/services/notifications.service";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export function useNotificationsColumns(
  handlers: NotificationRowActionHandlers,
  options?: { showUserColumn?: boolean },
) {
  const { t, locale } = useLocale();
  const showUserColumn = options?.showUserColumn ?? false;

  return useMemo(
    () =>
      [
        ...(showUserColumn
          ? [
              createColumn<Notification>({
                id: "user",
                labelKey: "notifications.columns.user",
                enableSorting: false,
                enableColumnFilter: false,
                cell: ({ row }) => {
                  const user = row.original.user;
                  if (!user) {
                    return <span className="text-muted-foreground">—</span>;
                  }
                  const name = `${user.name} ${user.surname}`.trim();
                  return (
                    <div className="flex min-w-0 flex-col">
                      <span className="truncate font-medium">
                        {name || "—"}
                      </span>
                      <span className="text-muted-foreground truncate text-xs">
                        {user.email}
                      </span>
                    </div>
                  );
                },
              }),
            ]
          : []),
        createColumn<Notification>({
          accessorKey: "title",
          labelKey: "notifications.columns.title",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.title}</span>
          ),
        }),
        createColumn<Notification>({
          accessorKey: "status",
          labelKey: "notifications.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          gridSecondary: true,
          filterOptions: NOTIFICATION_STATUS_VALUES.map((value) => ({
            value,
            labelKey: `notifications.status.${value}`,
            label: value,
          })),
          cell: ({ row }) => {
            const key = `notifications.status.${row.original.status}`;
            const label = t(key);
            return (
              <StatusChip
                label={label === key ? row.original.status : label}
                tone={statusTone(row.original.status)}
              />
            );
          },
        }),
        createColumn<Notification>({
          accessorKey: "priority",
          labelKey: "notifications.columns.priority",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) => {
            const key = `notifications.priority.${row.original.priority}`;
            const label = t(key);
            return (
              <StatusChip
                label={label === key ? row.original.priority : label}
                tone={priorityTone(row.original.priority)}
              />
            );
          },
        }),
        createColumn<Notification>({
          accessorKey: "template_code",
          labelKey: "notifications.columns.template_code",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.template_code ? (
              <span className="font-mono text-xs">
                {row.original.template_code}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Notification>({
          accessorKey: "recipient",
          labelKey: "notifications.columns.recipient",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.recipient ?? (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Notification>({
          accessorKey: "created_at",
          labelKey: "notifications.columns.created_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<Notification>({
          id: "actions",
          labelKey: "notifications.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <NotificationRowActionsMenu
              notification={row.original}
              handlers={handlers}
            />
          ),
        }),
      ] as ColumnDef<Notification, unknown>[],
    [handlers, locale, showUserColumn, t],
  );
}
