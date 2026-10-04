import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Announcement = Schemas["Announcement"];
export type AnnouncementAudience = Schemas["AnnouncementAudience"];
export type AnnouncementAudienceTarget =
  Schemas["AnnouncementAudienceTarget"];
export type AnnouncementInput = Schemas["AnnouncementInput"];
export type AnnouncementLocaleInput = Schemas["AnnouncementLocaleInput"];
export type AnnouncementReadReport = Schemas["AnnouncementReadReport"];
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
  reads(uuid: string, params = { limit: 200, offset: 0 }) {
    return platformRequest<AnnouncementReadReport>(
      "GET",
      `/v1/announcements/${enc(uuid)}/reads`,
      { query: params },
    );
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
};
