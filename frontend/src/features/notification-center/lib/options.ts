import { SUPPORTED_LOCALES } from "@/config/i18n";
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

// K10 + TEC-1233: the app languages (backend i18n.Supported).
export const LANGUAGES = SUPPORTED_LOCALES;

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
