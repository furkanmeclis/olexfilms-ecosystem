"use client";

import { useQueryClient } from "@tanstack/react-query";

import { notificationsKeys } from "@/features/notifications/hooks/query-keys";
import {
  notificationsService,
  type Notification,
  type NotificationListResult,
  type UnreadCount,
} from "@/features/notifications/services/notifications.service";
import { useAppMutation } from "@/lib/query/mutation";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

function patchNotificationInCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  uuid: string,
  patch: Partial<Notification>,
) {
  const patchList = (current: NotificationListResult | undefined) => {
    if (!current) return current;
    return {
      ...current,
      items: current.items.map((item) =>
        item.uuid === uuid ? { ...item, ...patch } : item,
      ),
    };
  };

  queryClient.setQueriesData<NotificationListResult>(
    { queryKey: notificationsKeys.platformLists() },
    patchList,
  );
  queryClient.setQueriesData<NotificationListResult>(
    { queryKey: notificationsKeys.inbox() },
    patchList,
  );
}

function bumpUnreadCount(
  queryClient: ReturnType<typeof useQueryClient>,
  delta: number,
) {
  queryClient.setQueriesData<UnreadCount>(
    { queryKey: notificationsKeys.unreadCount() },
    (current) => {
      if (!current) return current;
      return {
        ...current,
        count: Math.max(0, current.count + delta),
      };
    },
  );
}

function invalidateInbox(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: notificationsKeys.inbox() });
  void queryClient.invalidateQueries({
    queryKey: notificationsKeys.unreadCount(),
  });
  void queryClient.invalidateQueries({
    queryKey: notificationsKeys.platformLists(),
  });
}

export function useMarkNotificationRead(options?: { silent?: boolean }) {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  const silent = options?.silent ?? false;

  return useAppMutation({
    mutationFn: (uuid: string) => notificationsService.markRead(uuid),
    onMutate: async (uuid) => {
      await queryClient.cancelQueries({ queryKey: notificationsKeys.all });
      patchNotificationInCaches(queryClient, uuid, {
        status: "read",
        read_at: new Date().toISOString(),
      });
      bumpUnreadCount(queryClient, -1);
    },
    onError: () => {
      invalidateInbox(queryClient);
    },
    onSuccess: (notification) => {
      queryClient.setQueryData(
        notificationsKeys.detail(notification.uuid),
        notification,
      );
      patchNotificationInCaches(queryClient, notification.uuid, notification);
      if (!silent) {
        appToast.success(t("notifications.toast.marked_read"));
      }
    },
    onSettled: (_data, _error, uuid) => {
      void queryClient.invalidateQueries({
        queryKey: notificationsKeys.detail(uuid),
      });
      invalidateInbox(queryClient);
    },
  });
}

export function useMarkAllNotificationsRead() {
  const queryClient = useQueryClient();
  const { t } = useLocale();

  return useAppMutation({
    mutationFn: () => notificationsService.markAllRead(),
    onSuccess: () => {
      invalidateInbox(queryClient);
      appToast.success(t("notifications.toast.marked_all_read"));
    },
  });
}
