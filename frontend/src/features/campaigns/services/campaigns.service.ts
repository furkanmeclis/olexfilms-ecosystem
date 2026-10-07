import type { components } from "@/generated/api";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Campaign = Schemas["Campaign"];
export type CampaignStatus = Schemas["CampaignStatus"];
export type CampaignChannel = Schemas["CampaignChannel"];
export type CampaignAudienceFilter = Schemas["CampaignAudienceFilter"];
export type CampaignAudienceType = CampaignAudienceFilter["audience_type"];
export type CampaignInput = Schemas["CampaignInput"];
export type CampaignPatch = Schemas["CampaignPatch"];
export type CampaignContent = Schemas["CampaignContent"];
export type CampaignContentInput = Schemas["CampaignContentInput"];
export type CampaignMedia = Schemas["CampaignMedia"];
export type CampaignEvent = Schemas["CampaignEvent"];
export type CampaignPreview = Schemas["CampaignPreview"];
export type CampaignRecipientStatus = "pending" | "sent" | "failed" | "skipped";
export type CampaignRecipient = {
  uuid: string;
  name: string;
  phone: string | null;
  email: string | null;
  locale: LocaleCode;
  channel: CampaignChannel;
  status: CampaignRecipientStatus;
  sent_at: string | null;
  failure_reason: string | null;
  created_at: string;
};
export type LocaleCode = Schemas["LocaleCode"];

/**
 * GET /v1/campaigns params (TEC-405): `status` and `channel` are CSV,
 * `scheduled_from` / `scheduled_to` dates, `sort` one of created_at,
 * scheduled_at, name, status.
 */
export type CampaignListQuery = {
  status?: string;
  channel?: string;
  scheduled_from?: string;
  scheduled_to?: string;
  q?: string;
  sort?: string;
  limit: number;
  offset: number;
};

/** GET /v1/campaigns/approvals params (TEC-406). */
export type CampaignApprovalQuery = {
  channel?: string;
  organization_uuid?: string;
  q?: string;
  sort?: string;
  limit: number;
  offset: number;
};

/**
 * GET /v1/campaigns/{uuid}/recipients params: `status`, `channel` and
 * `locale` CSV, `sent_from` / `sent_to` dates, `sort` one of created_at,
 * sent_at, status, channel, locale.
 */
export type CampaignRecipientQuery = {
  status?: string;
  channel?: string;
  locale?: string;
  sent_from?: string;
  sent_to?: string;
  sort?: string;
  limit: number;
  offset: number;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;
const base = (uuid: string) => `/v1/campaigns/${enc(uuid)}`;

/** Recipient list export (ioengine resource `campaigns.recipients`). */
export function campaignRecipientsExportPath(uuid: string) {
  return `${base(uuid)}/recipients/export`;
}

export const campaignsService = {
  list(params: CampaignListQuery) {
    return platformRequest<Page<Campaign>>("GET", "/v1/campaigns", {
      query: params,
    });
  },
  approvals(params: CampaignApprovalQuery) {
    return platformRequest<Page<Campaign>>("GET", "/v1/campaigns/approvals", {
      query: params,
    });
  },
  get(uuid: string) {
    return platformRequest<Campaign>("GET", base(uuid));
  },
  create(body: CampaignInput) {
    return platformRequest<Campaign>("POST", "/v1/campaigns", { body });
  },
  update(uuid: string, body: CampaignPatch) {
    return platformRequest<Campaign>("PATCH", base(uuid), { body });
  },
  remove(uuid: string) {
    return platformRequest<void>("DELETE", base(uuid));
  },
  putContent(uuid: string, locale: LocaleCode, body: CampaignContentInput) {
    return platformRequest<CampaignContent>(
      "PUT",
      `${base(uuid)}/contents/${enc(locale)}`,
      { body },
    );
  },
  addMedia(uuid: string, locale: LocaleCode, file: File) {
    const form = new FormData();
    form.append("file", file);
    return platformFormRequest<CampaignMedia>(
      "POST",
      `${base(uuid)}/contents/${enc(locale)}/media`,
      form,
    );
  },
  deleteMedia(uuid: string, locale: LocaleCode, media: string) {
    return platformRequest<void>(
      "DELETE",
      `${base(uuid)}/contents/${enc(locale)}/media/${enc(media)}`,
    );
  },
  preview(uuid: string) {
    return platformRequest<CampaignPreview>("POST", `${base(uuid)}/preview`);
  },
  submit(uuid: string) {
    return platformRequest<Campaign>("POST", `${base(uuid)}/submit`);
  },
  /** `scheduled_at` local date-time in the organization time zone. */
  schedule(uuid: string, scheduledAt: string) {
    return platformRequest<Campaign>("POST", `${base(uuid)}/schedule`, {
      body: { scheduled_at: scheduledAt },
    });
  },
  sendNow(uuid: string) {
    return platformRequest<Campaign>("POST", `${base(uuid)}/send-now`);
  },
  cancel(uuid: string) {
    return platformRequest<Campaign>("POST", `${base(uuid)}/cancel`);
  },
  approve(uuid: string, reason?: string) {
    return platformRequest<Campaign>("POST", `${base(uuid)}/approve`, {
      body: reason ? { reason } : {},
    });
  },
  reject(uuid: string, reason: string) {
    return platformRequest<Campaign>("POST", `${base(uuid)}/reject`, {
      body: { reason },
    });
  },
  requestChanges(uuid: string, reason: string) {
    return platformRequest<Campaign>("POST", `${base(uuid)}/request-changes`, {
      body: { reason },
    });
  },
  recipients(uuid: string, params: CampaignRecipientQuery) {
    return platformRequest<Page<CampaignRecipient>>(
      "GET",
      `${base(uuid)}/recipients`,
      { query: params },
    );
  },
};

export const campaignKeys = {
  all: ["campaigns"] as const,
  lists: ["campaigns", "list"] as const,
  list: (params: CampaignListQuery) => ["campaigns", "list", params] as const,
  approvals: (params: CampaignApprovalQuery) =>
    ["campaigns", "approvals", params] as const,
  detail: (uuid: string) => ["campaigns", "detail", uuid] as const,
  preview: (uuid: string) => ["campaigns", "preview", uuid] as const,
  recipients: (uuid: string, params: CampaignRecipientQuery) =>
    ["campaigns", "recipients", uuid, params] as const,
};
