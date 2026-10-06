"use client";

import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceIOToolbar } from "@/features/io";
import type { ResourceMeta } from "@/features/io/types";
import {
  NotificationAudienceFilter,
  type NotificationAudience,
} from "@/features/notifications/components/notification-audience-filter";
import { NotificationDetailDrawer } from "@/features/notifications/components/notification-detail-drawer";
import { useNotificationsColumns } from "@/features/notifications/components/notifications-columns";
import type { NotificationRowActionHandlers } from "@/features/notifications/components/notification-row-actions";
import { NOTIFICATION_INBOX_CHANNEL } from "@/features/notifications/constants";
import { useNotificationRealtimeInvalidate } from "@/features/notifications/hooks/use-notification-realtime";
import {
  usePlatformNotificationsList,
  usePlatformNotificationsMeta,
} from "@/features/notifications/hooks/use-notifications-query";
import type {
  ListPlatformNotificationsParams,
  Notification,
} from "@/features/notifications/services/notifications.service";
import { useLocale } from "@/providers/locale-provider";

export const PLATFORM_NOTIFICATIONS_PERSIST_KEY = "platform-notifications-v4";

/** The page opened on the in-app inbox before the channel facet existed. */
const DEFAULT_CHANNEL_FILTER = [
  { id: "channel", value: [NOTIFICATION_INBOX_CHANNEL] },
];

export function NotificationsPage() {
  const { t } = useLocale();

  useNotificationRealtimeInvalidate();

  const [selected, setSelected] = useState<Notification | null>(null);
  const [audience, setAudience] = useState<NotificationAudience>({
    scope: "me",
  });

  const openDetail = useCallback((notification: Notification) => {
    setSelected(notification);
  }, []);

  const rowHandlers = useMemo<NotificationRowActionHandlers>(
    () => ({
      onView: openDetail,
    }),
    [openDetail],
  );

  const columns = useNotificationsColumns(rowHandlers, {
    showUserColumn: audience.scope === "all",
  });

  const metaQuery = usePlatformNotificationsMeta(true);
  const meta = metaQuery.data as ResourceMeta | undefined;

  // Column meta drives the params: status / channel / priority (CSV),
  // created (created_from / created_to).
  const listState = useServerListState({
    columns,
    initialSort: meta?.default_sort ?? "-created_at",
    initialPageSize: 20,
    persistKey: PLATFORM_NOTIFICATIONS_PERSIST_KEY,
    initialColumnFilters: DEFAULT_CHANNEL_FILTER,
  });

  const listParams = useMemo<ListPlatformNotificationsParams>(
    () => ({
      ...listState.params,
      scope: audience.scope === "all" ? "all" : "me",
      user_uuid: audience.scope === "user" ? audience.userUuid : undefined,
    }),
    [audience, listState.params],
  );

  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: listParams.q,
      sort: listParams.sort,
      scope: listParams.scope,
      user_uuid: listParams.user_uuid,
    }),
    [
      listState.filterParams,
      listParams.q,
      listParams.sort,
      listParams.scope,
      listParams.user_uuid,
    ],
  );

  const listEnabled = audience.scope !== "user" || Boolean(audience.userUuid);

  const listQuery = usePlatformNotificationsList(listParams, listEnabled);

  return (
    <EntityPage
      title={t("notifications.title")}
      description={t("notifications.description")}
      permission={permissions.notifications.platformRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("notifications.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("notifications.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listEnabled && listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("notifications.empty_title")}
        emptyDescription={t("notifications.empty_description")}
        rowCount={listQuery.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: PLATFORM_NOTIFICATIONS_PERSIST_KEY,
          // No bulk endpoint for notifications.
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <NotificationAudienceFilter
              value={audience}
              onChange={setAudience}
            />
            <ResourceIOToolbar
              resource="platform.notifications"
              query={exportQuery}
              capabilities={meta?.capabilities}
            />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={!listEnabled || listQuery.isFetching}
            />
          </>
        }
      />

      <NotificationDetailDrawer
        notification={selected}
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) setSelected(null);
        }}
      />
    </EntityPage>
  );
}
