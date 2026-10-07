import { SUPPORTED_LOCALES } from "@/config/i18n";
import type {
  Campaign,
  CampaignAudienceType,
  CampaignChannel,
  CampaignRecipientStatus,
  CampaignStatus,
  LocaleCode,
} from "@/features/campaigns/services/campaigns.service";

export const CAMPAIGN_PAGE_SIZE = 20;

export const CAMPAIGN_STATUSES: readonly CampaignStatus[] = [
  "draft",
  "pending_approval",
  "approved",
  "scheduled",
  "sending",
  "sent",
  "partially_failed",
  "cancelled",
  "rejected",
];

/** Campaign channels (TEC-405). There is no SMS channel. */
export const CAMPAIGN_CHANNELS: readonly CampaignChannel[] = [
  "push",
  "whatsapp",
  "email",
];

export const CAMPAIGN_RECIPIENT_STATUSES: readonly CampaignRecipientStatus[] = [
  "pending",
  "sent",
  "failed",
  "skipped",
];

export const CAMPAIGN_LOCALES = SUPPORTED_LOCALES as readonly LocaleCode[];

export const WARRANTY_STATUSES = ["active", "expiring", "expired"] as const;

/** Before sending a campaign can still be cancelled (TEC-406). */
const CANCELLABLE: readonly CampaignStatus[] = [
  "draft",
  "pending_approval",
  "approved",
  "scheduled",
  "sending",
];

export function canCancel(status: CampaignStatus) {
  return CANCELLABLE.includes(status);
}

/** Name, channels, audience and contents change while draft (TEC-405). */
export function isEditable(status: CampaignStatus) {
  return status === "draft";
}

export function canSchedule(status: CampaignStatus) {
  return status === "approved" || status === "scheduled";
}

export type Tone = "default" | "success" | "warning" | "danger";

export function campaignStatusTone(status: CampaignStatus): Tone {
  switch (status) {
    case "sent":
    case "approved":
      return "success";
    case "pending_approval":
    case "scheduled":
    case "sending":
      return "warning";
    case "partially_failed":
    case "rejected":
      return "danger";
    default:
      return "default";
  }
}

export function recipientStatusTone(status: CampaignRecipientStatus): Tone {
  switch (status) {
    case "sent":
      return "success";
    case "failed":
      return "danger";
    case "pending":
      return "warning";
    default:
      return "default";
  }
}

/**
 * Audience types per organization type (TEC-405): a dealer targets its
 * customers, a distributor customers or its dealers' users, the center
 * every type.
 */
export function audienceTypesFor(
  orgType: string | undefined,
): CampaignAudienceType[] {
  if (orgType === "center") {
    return ["customers", "dealer_users", "distributor_users"];
  }
  if (orgType === "distributor") return ["customers", "dealer_users"];
  return ["customers"];
}

/**
 * Only the center plans on its own (its submission is approved at once);
 * a dealer or distributor sends the campaign for approval (TEC-406, S4).
 */
export function plansDirectly(orgType: string | undefined) {
  return orgType === "center";
}

/** Character limits per channel (TEC-405, counted in code points). */
export const CHANNEL_LIMITS = {
  push: { title: 65, body: 240 },
  whatsapp: { body: 4096 },
  email: { title: 150 },
} as const;

export function charCount(value: string) {
  return [...value].length;
}

export type ContentField = "title" | "body";

export type ContentIssue = {
  channel: CampaignChannel;
  field: ContentField;
  kind: "required" | "too_long";
  limit?: number;
};

/**
 * Problems of one locale's content for the selected channels, the same
 * rules the API applies (push: title + body; e-mail: subject + body;
 * WhatsApp: body).
 */
export function contentIssues(
  channels: readonly CampaignChannel[],
  content: { title: string; body: string },
): ContentIssue[] {
  const issues: ContentIssue[] = [];
  const title = content.title.trim();
  const body = content.body.trim();
  for (const channel of channels) {
    const limits: { title?: number; body?: number } = CHANNEL_LIMITS[channel];
    const needsTitle = channel !== "whatsapp";
    if (needsTitle && !title) {
      issues.push({ channel, field: "title", kind: "required" });
    }
    if (!body) issues.push({ channel, field: "body", kind: "required" });
    if (limits.title && charCount(title) > limits.title) {
      issues.push({
        channel,
        field: "title",
        kind: "too_long",
        limit: limits.title,
      });
    }
    if (limits.body && charCount(body) > limits.body) {
      issues.push({
        channel,
        field: "body",
        kind: "too_long",
        limit: limits.body,
      });
    }
  }
  return issues;
}

/**
 * Locales that need a tab: every audience locale from the preview plus the
 * locales that already have content.
 */
export function requiredLocales(
  previewLocales: readonly { locale: LocaleCode }[],
  contentLocales: readonly LocaleCode[],
): LocaleCode[] {
  const set = new Set<LocaleCode>([
    ...previewLocales.map((l) => l.locale),
    ...contentLocales,
  ]);
  return CAMPAIGN_LOCALES.filter((l) => set.has(l));
}

/** Pending recipients: everyone not yet sent, failed or skipped. */
export function campaignStats(c: Campaign) {
  const done = c.recipients_sent + c.recipients_failed + c.recipients_skipped;
  const pending = Math.max(0, c.recipients_total - done);
  const attempted = c.recipients_sent + c.recipients_failed;
  const successRate =
    attempted > 0 ? Math.round((c.recipients_sent / attempted) * 100) : null;
  return { pending, successRate };
}

/**
 * Value for an `<input type="datetime-local">` showing `iso` in `timeZone`
 * (YYYY-MM-DDTHH:MM). The API reads such a local value in the
 * organization time zone.
 */
export function toZonedInput(iso: string, timeZone: string) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(iso));
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  return `${get("year")}-${get("month")}-${get("day")}T${get("hour")}:${get("minute")}`;
}
