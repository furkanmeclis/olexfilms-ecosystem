import type { ServerListParams } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type AISettings = Schemas["AISettings"];
export type AISettingsUpdate = Schemas["AISettingsUpdate"];
export type AIToolInfo = Schemas["AIToolInfo"];
export type AIOrgQuota = Schemas["AIOrgQuota"];
export type AIOrgQuotaUpdate = Schemas["AIOrgQuotaUpdate"];
export type AIOrgType = Schemas["AIOrgRef"]["type"];
export type AIUsageRow = Schemas["AIUsageRow"];
export type AIUsageSummary = Schemas["AIUsageSummary"];
export type AIQuotaUsage = Schemas["AIQuotaUsage"];
export type AIUserRef = Schemas["AIUserRef"];

/**
 * GET /v1/platform/ai/orgs (TEC-389): `sort` usage | quota | name, `q`,
 * `org_type` CSV, `period` YYYY-MM.
 */
export type AIOrgQuotaListParams = ServerListParams & {
  org_type?: string;
  period?: string;
};

/**
 * GET /v1/ai/usage and /v1/platform/ai/usage: `sort` created_at | tokens,
 * `created_from` / `created_to`, `tokens_min` / `tokens_max` (TEC-391),
 * CSV `channel`, `purpose`, `pool`, `model`, `user` and, on the platform
 * list, `organization`.
 */
export type AIUsageListParams = ServerListParams & {
  created_from?: string;
  created_to?: string;
  tokens_min?: string;
  tokens_max?: string;
  channel?: string;
  purpose?: string;
  pool?: string;
  model?: string;
  user?: string;
  organization?: string;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** Where the usage report reads from: every organization or the active one. */
export type AIUsageScope = "platform" | "tenant";

export const AI_USAGE_EXPORT_PATHS: Record<AIUsageScope, string> = {
  platform: "/v1/platform/ai/usage/export",
  tenant: "/v1/ai/usage/export",
};

const enc = encodeURIComponent;

export const aiAdminService = {
  settings() {
    return platformRequest<AISettings>("GET", "/v1/platform/ai/settings");
  },
  updateSettings(body: AISettingsUpdate) {
    return platformRequest<AISettings>("PUT", "/v1/platform/ai/settings", {
      body,
    });
  },
  orgQuotas(params: AIOrgQuotaListParams) {
    return platformRequest<Page<AIOrgQuota>>("GET", "/v1/platform/ai/orgs", {
      query: params,
    });
  },
  updateOrgQuota(uuid: string, body: AIOrgQuotaUpdate) {
    return platformRequest<AIOrgQuota>(
      "PUT",
      `/v1/platform/ai/orgs/${enc(uuid)}`,
      { body },
    );
  },
  usage(scope: AIUsageScope, params: AIUsageListParams) {
    return platformRequest<Page<AIUsageRow>>(
      "GET",
      scope === "platform" ? "/v1/platform/ai/usage" : "/v1/ai/usage",
      { query: params },
    );
  },
  summary(period?: string) {
    return platformRequest<AIUsageSummary>("GET", "/v1/ai/usage/summary", {
      query: { period },
    });
  },
};
