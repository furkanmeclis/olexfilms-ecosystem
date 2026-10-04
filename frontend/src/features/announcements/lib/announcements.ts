import { SUPPORTED_LOCALES, type AppLocale } from "@/config/i18n";
import type {
  Announcement,
  AnnouncementAudience,
  AnnouncementAudienceTarget,
  LocaleCode,
} from "@/features/announcements/services/announcements.service";
import type { OrganizationType } from "@/lib/auth/types";

export const ANNOUNCEMENT_PAGE_SIZE = 20;
export const ANNOUNCEMENT_DASHBOARD_LIMIT = 3;
export const ANNOUNCEMENT_LOCALES = SUPPORTED_LOCALES as readonly LocaleCode[];

export type AudienceOption = {
  value: AnnouncementAudienceTarget;
  labelKey: string;
};

export type LocaleDraft = Record<LocaleCode, { title: string; body: string }>;

export type AnnouncementFormValues = {
  defaultLocale: LocaleCode;
  locales: LocaleDraft;
  audience: AnnouncementAudienceTarget;
  roleSlug: string;
  publishAt: string;
  expiresAt: string;
  pinned: boolean;
  notify: boolean;
};

export function emptyLocaleDraft(): LocaleDraft {
  return Object.fromEntries(
    ANNOUNCEMENT_LOCALES.map((locale) => [locale, { title: "", body: "" }]),
  ) as LocaleDraft;
}

export function defaultAnnouncementForm(
  locale: AppLocale,
  orgType?: OrganizationType,
): AnnouncementFormValues {
  const defaultLocale = ANNOUNCEMENT_LOCALES.includes(locale as LocaleCode)
    ? (locale as LocaleCode)
    : "tr";
  return {
    defaultLocale,
    locales: emptyLocaleDraft(),
    audience: orgType === "distributor" ? "subtree" : "all_network",
    roleSlug: "",
    publishAt: "",
    expiresAt: "",
    pinned: false,
    notify: false,
  };
}

export function audienceOptions(orgType?: OrganizationType): AudienceOption[] {
  if (orgType === "distributor") {
    return [{ value: "subtree", labelKey: "announcements.audience.subtree" }];
  }
  return [
    { value: "all_network", labelKey: "announcements.audience.all_network" },
    {
      value: "distributors",
      labelKey: "announcements.audience.distributors",
    },
    { value: "dealers", labelKey: "announcements.audience.dealers" },
    { value: "role", labelKey: "announcements.audience.role" },
  ];
}

export function buildAudience(
  values: AnnouncementFormValues,
): AnnouncementAudience[] {
  const item: AnnouncementAudience = { target_type: values.audience };
  if (values.audience === "role" && values.roleSlug.trim()) {
    item.role_slug = values.roleSlug.trim();
  }
  return [item];
}

export function validateAnnouncementForm(
  values: AnnouncementFormValues,
): Record<string, string> {
  const errors: Record<string, string> = {};
  const current = values.locales[values.defaultLocale];
  if (!current.title.trim()) errors.title = "announcements.errors.title";
  if (!current.body.trim()) errors.body = "announcements.errors.body";
  if (values.audience === "role" && !values.roleSlug.trim()) {
    errors.role = "announcements.errors.role";
  }
  if (
    values.publishAt &&
    values.expiresAt &&
    new Date(values.expiresAt).getTime() < new Date(values.publishAt).getTime()
  ) {
    errors.expiresAt = "announcements.errors.expires_before_publish";
  }
  return errors;
}

export function toIsoOrNull(value: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date.toISOString();
}

export function isUnread(announcement: Announcement): boolean {
  return announcement.status === "published" && !announcement.read_at;
}

export function sortedAnnouncements(items: Announcement[]): Announcement[] {
  return [...items].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    if (isUnread(a) !== isUnread(b)) return isUnread(a) ? -1 : 1;
    return (b.publish_at ?? b.created_at ?? "").localeCompare(
      a.publish_at ?? a.created_at ?? "",
    );
  });
}

export function readRatePercent(readRate: number): number {
  return Math.round(readRate * 100);
}
