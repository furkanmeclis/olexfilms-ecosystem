"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { CHANNELS } from "@/features/notification-center/lib/options";
import {
  NOTIFICATION_PRIORITY_VALUES,
  NOTIFICATION_STATUS_VALUES,
  priorityTone,
  statusTone,
} from "@/features/notifications/constants";
import {
  NotificationRowActionsMenu,
  type NotificationRowActionHandlers,
} from "@/features/notifications/components/notification-row-actions";
import type { Notification } from "@/features/notifications/services/notifications.service";
import { useLocale } from "@/providers/locale-provider";

export function useNotificationsColumns(
  handlers: NotificationRowActionHandlers,
  options?: { showUserColumn?: boolean },
) {
  const { t, format } = useLocale();
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
          // Not in the backend sort whitelist; the toolbar search (`q`)
          // covers title, body, template code and recipient.
          enableSorting: false,
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
          param: "status",
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
          filterVariant: "faceted",
          param: "priority",
          filterOptions: NOTIFICATION_PRIORITY_VALUES.map((value) => ({
            value,
            labelKey: `notifications.priority.${value}`,
            label: value,
          })),
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
          accessorKey: "channel",
          labelKey: "notifications.columns.channel",
          enableSorting: true,
          filterVariant: "faceted",
          param: "channel",
          filterOptions: CHANNELS.map((value) => ({
            value,
            labelKey: `notifications.center.channels.${value}`,
            label: value,
          })),
          cell: ({ row }) => {
            const key = `notifications.center.channels.${row.original.channel}`;
            const label = t(key);
            return label === key ? row.original.channel : label;
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
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<Notification>({
          accessorKey: "sent_at",
          labelKey: "notifications.fields.sent_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.sent_at ? (
              format.dateTime(row.original.sent_at)
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
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
    [handlers, showUserColumn, t, format],
  );
}
