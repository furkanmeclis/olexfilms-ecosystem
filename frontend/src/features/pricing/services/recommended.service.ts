import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type RecommendedPriceRef = Schemas["RecommendedPriceRef"];
export type RecommendedPriceVersion = Schemas["RecommendedPriceVersion"];
export type CurrentRecommendedPrice = Schemas["CurrentRecommendedPrice"];
export type RecommendedPublishRequest = Schemas["RecommendedPublishRequest"];
export type RecommendedPublishResult = Schemas["RecommendedPublishResult"];
export type RecommendedPriceSettings = Schemas["RecommendedPriceSettings"];
export type PriceDisciplineRow = Schemas["PriceDisciplineRow"];
export type PriceDisciplineSummary = Schemas["PriceDisciplineSummary"];
export type DisciplineCountry = PriceDisciplineSummary["countries"][number];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** List query: paging, `sort`, `q` and the endpoint's filters. */
export type ListQuery = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
} & Record<string, string | number | boolean | undefined>;

/** Query params as platformRequest takes them (booleans as text). */
function toQuery(params: ListQuery) {
  const query: Record<string, string | number | undefined> = {};
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") continue;
    query[key] = typeof value === "boolean" ? String(value) : value;
  }
  return query;
}

/** Import / export endpoints of the recommended price list (TEC-506). */
export const RECOMMENDED_IMPORT_PATHS = {
  upload: "/v1/tenant/pricing/recommended/import",
  sample: "/v1/tenant/pricing/recommended/import/sample",
};
export const DISCIPLINE_EXPORT_PATH = "/v1/pricing/discipline/export";

/**
 * Recommended retail prices (TEC-506). Publishing is the center's and needs
 * a recent step-up; platformRequest retries through the step-up engine.
 */
export const recommendedService = {
  settings() {
    return platformRequest<RecommendedPriceSettings>(
      "GET",
      "/v1/tenant/pricing/recommended/settings",
    );
  },

  current(params: ListQuery) {
    return platformRequest<Page<CurrentRecommendedPrice>>(
      "GET",
      "/v1/tenant/pricing/recommended/current",
      { query: toQuery(params) },
    );
  },

  versions(params: ListQuery) {
    return platformRequest<Page<RecommendedPriceVersion>>(
      "GET",
      "/v1/tenant/pricing/recommended/versions",
      { query: toQuery(params) },
    );
  },

  publish(body: RecommendedPublishRequest) {
    return platformRequest<RecommendedPublishResult>(
      "POST",
      "/v1/tenant/pricing/recommended/publish",
      { body },
    );
  },

  discipline(params: ListQuery) {
    return platformRequest<Page<PriceDisciplineRow>>(
      "GET",
      "/v1/pricing/discipline",
      { query: toQuery(params) },
    );
  },

  disciplineSummary(date?: string) {
    return platformRequest<PriceDisciplineSummary>(
      "GET",
      "/v1/pricing/discipline/summary",
      { query: { date: date || undefined } },
    );
  },
};

export const pricingKeys = {
  all: ["pricing"] as const,
  settings: ["pricing", "recommended", "settings"] as const,
  current: (params: unknown) =>
    ["pricing", "recommended", "current", params] as const,
  versions: (params: unknown) =>
    ["pricing", "recommended", "versions", params] as const,
  discipline: (params: unknown) => ["pricing", "discipline", params] as const,
  disciplineSummary: (date: string) =>
    ["pricing", "discipline", "summary", date] as const,
};
