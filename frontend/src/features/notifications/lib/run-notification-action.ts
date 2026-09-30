import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import type { NotificationAction } from "@/features/notifications/lib/notification-action";
import { notificationsService } from "@/features/notifications/services/notifications.service";

export type NotificationActionSource = {
  uuid?: string;
  signed_action_url?: string | null;
};

type RunNotificationActionOptions = {
  action: NotificationAction;
  navigate: (href: string) => void;
  notification?: NotificationActionSource | null;
};

export async function runNotificationAction({
  action,
  navigate,
  notification,
}: RunNotificationActionOptions) {
  if (notification) {
    await notificationsService.redeemSignedAction(notification);
  }
  if (action.kind === "download") {
    const { blob, filename } = await platformDownloadFile(action.href);
    triggerBrowserDownload(blob, filename ?? "export");
    return;
  }
  if (action.kind === "external") {
    window.open(action.href, "_blank", "noopener,noreferrer");
    return;
  }
  navigate(action.href);
}
