import { isLiveSurfaceChannel } from "@/features/notifications/constants";
import { notificationsService } from "@/features/notifications/services/notifications.service";
import type { RealtimeMessage } from "@/lib/realtime";

export type LiveNotification = {
  uuid?: string;
  channel?: string;
  title: string;
  body?: string;
  status?: string;
  priority?: string;
  action_url?: string;
  signed_action_url?: string;
  template_code?: string;
  payload?: Record<string, unknown>;
};

function asString(value: unknown) {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

function fromRecord(record: Record<string, unknown>): LiveNotification | null {
  const title = asString(record.title);
  if (!title) return null;
  return {
    uuid: asString(record.uuid),
    channel: asString(record.channel),
    title,
    body: asString(record.body),
    status: asString(record.status),
    priority: asString(record.priority),
    action_url: asString(record.action_url),
    signed_action_url: asString(record.signed_action_url),
    template_code: asString(record.template_code),
    payload: asRecord(record.payload) ?? undefined,
  };
}

export function liveNotificationFromMessage(
  message: RealtimeMessage,
): LiveNotification | null {
  const data = message.data;
  const payload = asRecord(data.payload);
  const candidates = [
    asRecord(data.notification),
    payload ? asRecord(payload.notification) : null,
    payload,
  ];

  for (const record of candidates) {
    if (!record) continue;
    const parsed = fromRecord(record);
    if (parsed && isLiveSurfaceChannel(parsed.channel)) return parsed;
  }

  return null;
}

export async function enrichLiveNotification(
  notification: LiveNotification,
): Promise<LiveNotification> {
  if (
    !notification.uuid ||
    (notification.action_url && notification.signed_action_url)
  ) {
    return notification;
  }
  try {
    const full = await notificationsService.get(notification.uuid);
    return {
      ...notification,
      body: notification.body || full.body,
      priority: notification.priority || full.priority,
      action_url: notification.action_url || full.action_url,
      signed_action_url:
        notification.signed_action_url || full.signed_action_url,
      template_code: notification.template_code || full.template_code,
      channel: notification.channel || full.channel,
      payload:
        notification.payload ||
        (full.payload as Record<string, unknown> | undefined),
    };
  } catch {
    return notification;
  }
}

export function liveToastDuration(priority?: string) {
  switch (priority) {
    case "low":
      return 7_000;
    case "high":
      return 14_000;
    case "critical":
      return Infinity;
    default:
      return 10_000;
  }
}
