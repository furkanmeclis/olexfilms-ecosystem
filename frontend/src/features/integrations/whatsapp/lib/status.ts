import type { BadgeProps } from "@/components/ui/badge";

export const WHATSAPP_STATUSES = [
  "unknown",
  "disconnected",
  "connecting",
  "qr",
  "connected",
  "logged_out",
  "banned",
] as const;

export type WhatsAppStatusKey = (typeof WHATSAPP_STATUSES)[number];

/** Badge tone of a gateway status. */
export function statusVariant(status: string): BadgeProps["variant"] {
  switch (status) {
    case "connected":
      return "success";
    case "connecting":
    case "qr":
      return "warning";
    case "logged_out":
    case "banned":
      return "danger";
    default:
      return "secondary";
  }
}

/** i18n key of a status; unknown values map to "unknown". */
export function statusLabelKey(status: string): string {
  const key = (WHATSAPP_STATUSES as readonly string[]).includes(status)
    ? status
    : "unknown";
  return `integrations.whatsapp.status.${key}`;
}

/** QR polling runs while the session waits for a scan (2.5 s). */
export function qrPollInterval(polling: boolean, connected: boolean) {
  return polling && !connected ? 2500 : false;
}
