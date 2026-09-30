import { create } from "zustand";

import type { LiveNotification } from "@/features/notifications/lib/live-notification";
import { liveToastDuration } from "@/features/notifications/lib/live-notification";
import { notificationFingerprint } from "@/features/notifications/lib/notification-action";

export type LiveNotificationItem = LiveNotification & {
  id: string;
  durationMs: number;
};

type LiveNotificationState = {
  items: LiveNotificationItem[];
  push: (notification: LiveNotification) => string | null;
  dismiss: (id: string) => void;
};

const MAX_VISIBLE = 3;

export const useNotificationLiveStore = create<LiveNotificationState>(
  (set, get) => ({
    items: [],
    push: (notification) => {
      const fingerprint =
        notificationFingerprint(notification) ||
        `${notification.title}-${Date.now()}`;
      const existing = get().items.find((item) => item.id === fingerprint);
      if (existing) {
        set((state) => ({
          items: state.items.map((item) =>
            item.id === fingerprint
              ? {
                  ...item,
                  ...notification,
                  id: fingerprint,
                  durationMs: liveToastDuration(notification.priority),
                }
              : item,
          ),
        }));
        return fingerprint;
      }

      const next: LiveNotificationItem = {
        ...notification,
        id: fingerprint,
        durationMs: liveToastDuration(notification.priority),
      };

      set((state) => ({
        items: [next, ...state.items].slice(0, MAX_VISIBLE),
      }));
      return fingerprint;
    },
    dismiss: (id) =>
      set((state) => ({
        items: state.items.filter((item) => item.id !== id),
      })),
  }),
);
