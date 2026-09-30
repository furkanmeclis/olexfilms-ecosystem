"use client";

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { notificationsKeys } from "@/features/notifications/hooks/query-keys";
import { useRealtimeEvent } from "@/hooks/use-realtime";
import { realtimeConfig } from "@/config/realtime";
import { RealtimeEvents } from "@/lib/realtime/events";

const NOTIFICATION_INVALIDATE_EVENTS = new Set<string>([
  RealtimeEvents.NotificationQueued,
  RealtimeEvents.NotificationSent,
  RealtimeEvents.NotificationDelivered,
  RealtimeEvents.NotificationRead,
  RealtimeEvents.NotificationFailed,
  RealtimeEvents.NotificationCancelled,
  RealtimeEvents.NotificationItemCreated,
  RealtimeEvents.NotificationItemUpdated,
  RealtimeEvents.NotificationItemRead,
]);

/**
 * Invalidate notification queries on Centrifugo events.
 * When realtime is disabled, callers rely on poll / refetchOnWindowFocus.
 */
export function useNotificationRealtimeInvalidate(enabled = true) {
  const queryClient = useQueryClient();
  const active = enabled && realtimeConfig.enabled;

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: notificationsKeys.all });
  }, [queryClient]);

  useRealtimeEvent(
    "*",
    (message) => {
      if (!NOTIFICATION_INVALIDATE_EVENTS.has(message.type)) return;
      invalidate();
    },
    { enabled: active },
  );
}
