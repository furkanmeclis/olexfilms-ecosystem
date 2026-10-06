"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  CHANNELS,
  DELIVERY_STATUSES,
  deliveryStatusVariant,
} from "@/features/notification-center/lib/options";
import {
  notificationCenterService,
  type DeliveryFilter,
  type NotificationDelivery,
} from "@/features/notification-center/services/notification-center.service";
import { useLocale } from "@/providers/locale-provider";

export const DELIVERIES_PERSIST_KEY = "platform-notification-deliveries-v1";
const KEY = ["platform", "notification-center"] as const;

/**
 * Delivery log (`GET /v1/platform/notification-deliveries`): server-side
 * sort (created_at, status, channel, event_code), `q` on recipient email /
 * event code, CSV status + channel, event select and created date range.
 */
export function DeliveriesTable() {
  const { t, format } = useLocale();

  const events = useQuery({
    queryKey: [...KEY, "events"],
    queryFn: () => notificationCenterService.events(),
  });
  const eventOptions = useMemo(
    () =>
      (events.data ?? []).map((event) => ({
        value: event.code,
        label: event.code,
      })),
    [events.data],
  );

  const columns = useMemo<ColumnDef<NotificationDelivery, unknown>[]>(
    () => [
      createColumn<NotificationDelivery>({
        accessorKey: "created_at",
        labelKey: "notifications.center.created",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
      createColumn<NotificationDelivery>({
        accessorKey: "event_code",
        labelKey: "notifications.center.event",
        enableSorting: true,
        filterVariant: "select",
        param: "event_code",
        filterOptions: eventOptions,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-mono text-xs">{row.original.event_code}</span>
        ),
      }),
      createColumn<NotificationDelivery>({
        id: "recipient",
        accessorFn: (row) => row.user_email || row.user_uuid,
        labelKey: "notifications.center.recipient",
        enableSorting: false,
        gridSecondary: true,
      }),
      createColumn<NotificationDelivery>({
        accessorKey: "channel",
        labelKey: "notifications.center.channel",
        enableSorting: true,
        filterVariant: "faceted",
        param: "channel",
        filterOptions: CHANNELS.map((value) => ({
          value,
          labelKey: `notifications.center.channels.${value}`,
          label: value,
        })),
        cell: ({ row }) =>
          t(`notifications.center.channels.${row.original.channel}`),
      }),
      createColumn<NotificationDelivery>({
        accessorKey: "language",
        labelKey: "notifications.center.language",
        enableSorting: false,
        cell: ({ row }) => row.original.language || "—",
      }),
      createColumn<NotificationDelivery>({
        accessorKey: "status",
        labelKey: "notifications.center.status",
        enableSorting: true,
        filterVariant: "faceted",
        param: "status",
        filterOptions: DELIVERY_STATUSES.map((value) => ({
          value,
          labelKey: `notifications.center.statuses.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <span title={row.original.error || undefined}>
            <Badge variant={deliveryStatusVariant(row.original.status)}>
              {t(`notifications.center.statuses.${row.original.status}`)}
            </Badge>
          </span>
        ),
      }),
      createColumn<NotificationDelivery>({
        accessorKey: "attempts",
        labelKey: "notifications.center.attempts",
        enableSorting: false,
        defaultHidden: true,
      }),
      createColumn<NotificationDelivery>({
        accessorKey: "error",
        labelKey: "notifications.center.delivery_error",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.error ? (
            <span className="text-destructive line-clamp-2 text-xs">
              {row.original.error}
            </span>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
    ],
    [eventOptions, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 25,
    persistKey: DELIVERIES_PERSIST_KEY,
  });
  const filter = listState.params as DeliveryFilter;

  const page = useQuery({
    queryKey: [...KEY, "deliveries", filter],
    queryFn: () => notificationCenterService.deliveries(filter),
  });

  return (
    <EntityTable
      columns={columns}
      data={page.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={page.isLoading}
      isError={page.isError}
      errorTitle={t("notifications.center.error")}
      onRetry={() => void page.refetch()}
      emptyTitle={t("notifications.center.no_deliveries")}
      emptyDescription=""
      rowCount={page.data?.total ?? 0}
      state={listState.tableState}
      pageSizeOptions={[25, 50, 100]}
      features={{
        persistKey: DELIVERIES_PERSIST_KEY,
        // No retry / bulk endpoint for deliveries.
        rowSelection: false,
      }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void page.refetch()}
          refreshDisabled={page.isFetching}
        />
      }
    />
  );
}
