export const NOTIFICATION_INBOX_CHANNEL = "inapp" as const;
export const NOTIFICATION_LIVE_CHANNEL = "realtime" as const;

export const NOTIFICATION_CHANNEL_VALUES = [
  "email",
  "sms",
  "push",
  "inapp",
  "realtime",
] as const;

export type NotificationChannel = (typeof NOTIFICATION_CHANNEL_VALUES)[number];

export const NOTIFICATION_STATUS_VALUES = [
  "queued",
  "processing",
  "sent",
  "delivered",
  "read",
  "failed",
  "cancelled",
] as const;

export type NotificationStatus = (typeof NOTIFICATION_STATUS_VALUES)[number];

export const NOTIFICATION_PRIORITY_VALUES = [
  "low",
  "normal",
  "high",
  "critical",
] as const;

export type NotificationPriority =
  (typeof NOTIFICATION_PRIORITY_VALUES)[number];

export const NOTIFICATION_STATUS_TONE: Record<
  NotificationStatus,
  "success" | "danger" | "warning" | "default"
> = {
  queued: "default",
  processing: "warning",
  sent: "default",
  delivered: "success",
  read: "success",
  failed: "danger",
  cancelled: "default",
};

export const NOTIFICATION_PRIORITY_TONE: Record<
  NotificationPriority,
  "success" | "danger" | "warning" | "default"
> = {
  low: "default",
  normal: "default",
  high: "warning",
  critical: "danger",
};

/** Header inbox page size (recent ops feed). */
export const NOTIFICATION_INBOX_LIMIT = 15;

/** Statuses treated as unread for inbox row highlight. */
export const NOTIFICATION_UNREAD_STATUSES = [
  "queued",
  "processing",
  "sent",
  "delivered",
] as const satisfies readonly NotificationStatus[];

export function statusTone(status: string) {
  if (status in NOTIFICATION_STATUS_TONE) {
    return NOTIFICATION_STATUS_TONE[status as NotificationStatus];
  }
  return "default" as const;
}

export function priorityTone(priority: string) {
  if (priority in NOTIFICATION_PRIORITY_TONE) {
    return NOTIFICATION_PRIORITY_TONE[priority as NotificationPriority];
  }
  return "default" as const;
}

export function isInboxChannel(channel: string | undefined) {
  return channel === NOTIFICATION_INBOX_CHANNEL;
}

/** In-app inbox + Centrifugo live cards. Email/SMS/push stay off-screen. */
export function isLiveSurfaceChannel(channel: string | undefined) {
  if (!channel) return true;
  return (
    channel === NOTIFICATION_INBOX_CHANNEL ||
    channel === NOTIFICATION_LIVE_CHANNEL
  );
}

/** In-app notifications that `POST …/read` can mark. */
export function canMarkNotificationRead(notification: {
  channel: string;
  status: string;
  read_at?: string | null;
}) {
  if (!isInboxChannel(notification.channel)) return false;
  if (notification.read_at) return false;
  return notification.status !== "read";
}

export function isUnreadNotification(notification: {
  status: string;
  read_at?: string | null;
}) {
  if (notification.read_at) return false;
  return (NOTIFICATION_UNREAD_STATUSES as readonly string[]).includes(
    notification.status,
  );
}
