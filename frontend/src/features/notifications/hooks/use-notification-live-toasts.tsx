"use client";

import { realtimeConfig } from "@/config/realtime";
import { isLiveSurfaceChannel } from "@/features/notifications/constants";
import { useNotificationRealtimeInvalidate } from "@/features/notifications/hooks/use-notification-realtime";
import {
  enrichLiveNotification,
  liveNotificationFromMessage,
} from "@/features/notifications/lib/live-notification";
import { useNotificationLiveStore } from "@/features/notifications/lib/notification-live-store";
import { useRealtimeEvent } from "@/hooks/use-realtime";
import { RealtimeEvents } from "@/lib/realtime/events";

const LIVE_EVENT_TYPES = new Set<string>([
  RealtimeEvents.NotificationItemCreated,
]);

/**
 * Centrifugo live notifications land in the corner host.
 * Inbox / table stay in-app; realtime is the on-screen channel.
 */
export function useNotificationLiveToasts(enabled = true) {
  const active = enabled && realtimeConfig.enabled;
  const push = useNotificationLiveStore((state) => state.push);

  useNotificationRealtimeInvalidate(enabled);

  useRealtimeEvent(
    "*",
    (message) => {
      if (!LIVE_EVENT_TYPES.has(message.type)) return;
      const incoming = liveNotificationFromMessage(message);
      if (!incoming || !isLiveSurfaceChannel(incoming.channel)) return;

      void (async () => {
        const notification = await enrichLiveNotification(incoming);
        push(notification);
      })();
    },
    { enabled: active },
  );
}
