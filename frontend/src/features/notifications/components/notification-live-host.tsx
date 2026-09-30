"use client";

import { NotificationLiveCard } from "@/features/notifications/components/notification-live-card";
import { useNotificationLiveStore } from "@/features/notifications/lib/notification-live-store";

export function NotificationLiveHost() {
  const items = useNotificationLiveStore((state) => state.items);
  const dismiss = useNotificationLiveStore((state) => state.dismiss);

  if (items.length === 0) return null;

  return (
    <div
      className="pointer-events-none fixed end-4 bottom-4 z-[200] flex w-[min(24rem,calc(100vw-2rem))] flex-col-reverse gap-3 sm:end-6 sm:bottom-6"
      aria-label="Live notifications"
    >
      {items.map((item) => (
        <NotificationLiveCard key={item.id} item={item} onDismiss={dismiss} />
      ))}
    </div>
  );
}
