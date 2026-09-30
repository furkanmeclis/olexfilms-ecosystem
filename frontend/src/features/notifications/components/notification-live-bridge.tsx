"use client";

import { permissions } from "@/config/permissions";
import { NotificationLiveHost } from "@/features/notifications/components/notification-live-host";
import { useNotificationLiveToasts } from "@/features/notifications/hooks/use-notification-live-toasts";
import { usePermission } from "@/providers/permission-provider";

/** Listens for live notifications and renders the corner cards. */
export function NotificationLiveBridge() {
  const { can } = usePermission();
  useNotificationLiveToasts(can(permissions.notifications.read));
  return <NotificationLiveHost />;
}
