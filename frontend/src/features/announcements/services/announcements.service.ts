import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Announcement = Schemas["Announcement"];
export type AnnouncementAudience = Schemas["AnnouncementAudience"];
export type AnnouncementAudienceTarget = Schemas["AnnouncementAudienceTarget"];
export type AnnouncementInput = Schemas["AnnouncementInput"];
export type AnnouncementLocaleInput = Schemas["AnnouncementLocaleInput"];
export type AnnouncementReadReport = Schemas["AnnouncementReadReport"];
export type AnnouncementReadItem = Schemas["AnnouncementReadItem"];
export type AnnouncementUnreadCount = Schemas["AnnouncementUnreadCount"];
export type LocaleCode = Schemas["LocaleCode"];

export type AnnouncementPage = {
  items: Announcement[];
  total: number;
  limit: number;
  offset: number;
};

export type AnnouncementListQuery = {
  locale?: LocaleCode;
  unread_only?: boolean;
  limit: number;
  offset: number;
};

/** `GET /v1/announcements/manage` (authors, every status; TEC-367). */
export type AnnouncementManageQuery = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  /** CSV of draft, published, archived */
  status?: string;
  pinned?: string;
  publish_from?: string;
  publish_to?: string;
};

/** Page size of the read report (backend caps `limit` at 500). */
export const ANNOUNCEMENT_READS_PAGE_SIZE = 500;
/** Safety stop for very large audiences (500 × 40 = 20 000 readers). */
const ANNOUNCEMENT_READS_MAX_PAGES = 40;

const enc = encodeURIComponent;

export const announcementsService = {
  list(params: AnnouncementListQuery) {
    const query = {
      locale: params.locale,
      unread_only:
        params.unread_only === undefined
          ? undefined
          : params.unread_only
            ? "true"
            : "false",
      limit: params.limit,
      offset: params.offset,
    };
    return platformRequest<AnnouncementPage>("GET", "/v1/announcements", {
      query,
    });
  },
  manage(params: AnnouncementManageQuery) {
    return platformRequest<AnnouncementPage>(
      "GET",
      "/v1/announcements/manage",
      { query: params },
    );
  },
  unreadCount(locale?: LocaleCode) {
    return platformRequest<AnnouncementUnreadCount>(
      "GET",
      "/v1/announcements/unread-count",
      { query: { locale } },
    );
  },
  get(uuid: string, locale?: LocaleCode) {
    return platformRequest<Announcement>(
      "GET",
      `/v1/announcements/${enc(uuid)}`,
      { query: { locale } },
    );
  },
  create(body: AnnouncementInput) {
    return platformRequest<Announcement>("POST", "/v1/announcements", {
      body,
    });
  },
  update(uuid: string, body: AnnouncementInput) {
    return platformRequest<Announcement>(
      "PATCH",
      `/v1/announcements/${enc(uuid)}`,
      { body },
    );
  },
  putLocale(uuid: string, locale: LocaleCode, body: AnnouncementLocaleInput) {
    return platformRequest<Announcement>(
      "PUT",
      `/v1/announcements/${enc(uuid)}/locales/${enc(locale)}`,
      { body },
    );
  },
  publish(uuid: string) {
    return platformRequest<Announcement>(
      "POST",
      `/v1/announcements/${enc(uuid)}/publish`,
    );
  },
  archive(uuid: string) {
    return platformRequest<Announcement>(
      "POST",
      `/v1/announcements/${enc(uuid)}/archive`,
    );
  },
  pin(uuid: string, pinned: boolean) {
    return platformRequest<Announcement>(
      "POST",
      `/v1/announcements/${enc(uuid)}/pin`,
      { body: { pinned } },
    );
  },
  reads(
    uuid: string,
    params = { limit: ANNOUNCEMENT_READS_PAGE_SIZE, offset: 0 },
  ) {
    return platformRequest<AnnouncementReadReport>(
      "GET",
      `/v1/announcements/${enc(uuid)}/reads`,
      { query: params },
    );
  },
  /**
   * The whole read report: pages through `read_total` (the old single
   * request was clamped at 100 readers).
   */
  async readsAll(uuid: string): Promise<AnnouncementReadReport> {
    const first = await announcementsService.reads(uuid, {
      limit: ANNOUNCEMENT_READS_PAGE_SIZE,
      offset: 0,
    });
    const items = [...(first.items ?? [])];
    let pages = 1;
    while (
      items.length < first.read_total &&
      pages < ANNOUNCEMENT_READS_MAX_PAGES
    ) {
      const next = await announcementsService.reads(uuid, {
        limit: ANNOUNCEMENT_READS_PAGE_SIZE,
        offset: items.length,
      });
      const batch = next.items ?? [];
      if (!batch.length) break;
      items.push(...batch);
      pages += 1;
    }
    return { ...first, items, limit: items.length, offset: 0 };
  },
};

export const announcementKeys = {
  all: ["announcements"] as const,
  lists: () => [...announcementKeys.all, "list"] as const,
  list: (params: AnnouncementListQuery) =>
    [...announcementKeys.lists(), params] as const,
  details: () => [...announcementKeys.all, "detail"] as const,
  detail: (uuid: string, locale?: LocaleCode) =>
    [...announcementKeys.details(), uuid, locale ?? ""] as const,
  unreadCount: (locale?: LocaleCode) =>
    [...announcementKeys.all, "unread-count", locale ?? ""] as const,
  reads: (uuid: string) => [...announcementKeys.all, "reads", uuid] as const,
  manage: (params: AnnouncementManageQuery) =>
    [...announcementKeys.all, "manage", params] as const,
};
