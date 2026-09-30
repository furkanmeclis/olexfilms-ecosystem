"use client";

import { useQuery } from "@tanstack/react-query";

import {
  NOTIFICATION_INBOX_CHANNEL,
  NOTIFICATION_INBOX_LIMIT,
} from "@/features/notifications/constants";
import { notificationsKeys } from "@/features/notifications/hooks/query-keys";
import {
  notificationsService,
  type ListPlatformNotificationsParams,
} from "@/features/notifications/services/notifications.service";

export function usePlatformNotificationsList(
  params: ListPlatformNotificationsParams,
  enabled = true,
) {
  return useQuery({
    queryKey: notificationsKeys.platformList(params),
    queryFn: () => notificationsService.listPlatform(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function usePlatformNotificationsMeta(enabled = true) {
  return useQuery({
    queryKey: notificationsKeys.platformMeta(),
    queryFn: () => notificationsService.platformMeta(),
    enabled,
    staleTime: 5 * 60_000,
  });
}

export function useNotification(uuid: string | null, enabled = true) {
  return useQuery({
    queryKey: notificationsKeys.detail(uuid ?? ""),
    queryFn: () => notificationsService.get(uuid!),
    enabled: Boolean(uuid) && enabled,
  });
}

/** Personal inbox: actor-scoped in-app notifications. */
export function useNotificationsInbox(enabled = true) {
  return useQuery({
    queryKey: notificationsKeys.inbox(),
    queryFn: () =>
      notificationsService.listInbox({
        limit: NOTIFICATION_INBOX_LIMIT,
        offset: 0,
        sort: "-created_at",
        channel: NOTIFICATION_INBOX_CHANNEL,
      }),
    enabled,
    placeholderData: (previous) => previous,
    refetchOnWindowFocus: true,
  });
}

/** Personal inbox unread badge. */
export function useNotificationsUnreadCount(enabled = true) {
  return useQuery({
    queryKey: notificationsKeys.unreadCount(),
    queryFn: () => notificationsService.unreadCount(),
    enabled,
    placeholderData: (previous) => previous,
    refetchOnWindowFocus: true,
  });
}
