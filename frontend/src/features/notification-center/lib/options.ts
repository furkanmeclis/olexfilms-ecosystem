import type {
  NotificationChannel,
  NotificationDeliveryStatus,
  NotificationTemplateRole,
} from "@/features/notification-center/services/notification-center.service";

export const CHANNELS: NotificationChannel[] = [
  "inapp",
  "email",
  "webpush",
  "expo_push",
  "sms",
  "whatsapp",
];

export const ROLES: NotificationTemplateRole[] = [
  "generic",
  "customer",
  "dealer",
  "distributor",
  "center",
];

// K10: 12 languages + Arabic (backend i18n.Supported).
export const LANGUAGES = [
  "tr",
  "en",
  "bg",
  "de",
  "el",
  "uk",
  "ru",
  "fr",
  "es",
  "it",
  "zh-CN",
  "az",
  "ar",
] as const;

export const DELIVERY_STATUSES: NotificationDeliveryStatus[] = [
  "queued",
  "processing",
  "sent",
  "delivered",
  "failed",
  "skipped_disabled",
  "skipped_preference",
  "skipped_no_template",
  "skipped_no_recipient",
];

export function deliveryStatusVariant(
  status: NotificationDeliveryStatus,
): "default" | "secondary" | "danger" | "outline" {
  switch (status) {
    case "sent":
    case "delivered":
      return "default";
    case "failed":
      return "danger";
    case "queued":
    case "processing":
      return "secondary";
    default:
      return "outline";
  }
}
