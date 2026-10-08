import type { ServerListQuery } from "@/components/entity";
import { platformRequest } from "@/lib/api/platform-request";

/** Summary breakdowns of GET /v1/efficiency/summary (TEC-488). */
export type EfficiencyDimension =
  "dealer" | "staff" | "product" | "body_type" | "part";

/** Ratios and meters are decimal strings; null when nothing is expected. */
export type EfficiencySummaryRow = {
  dimension_key: string;
  dimension_label: string;
  services: number;
  meters: string;
  expected_meters: string | null;
  waste_ratio: string | null;
};

export type EfficiencyTrendRow = {
  month: string;
  services: number;
  meters: string;
  expected_meters: string | null;
  waste_ratio: string | null;
};

export type EfficiencyCompareBucket = "organization" | "subtree" | "network";

export type EfficiencyCompareRow = {
  bucket: EfficiencyCompareBucket;
  services: number;
  meters: string;
  expected_meters: string | null;
  waste_ratio: string | null;
};

export type EfficiencyRollRow = {
  uuid: string;
  barcode: string;
  product_name: string;
  initial_meters: string;
  consumed_meters: string;
  expected_meters: string;
  waste_meters: string;
  remaining_meters: string;
  service_count: number;
  last_used_at: string | null;
  waste_ratio: string | null;
};

export type EfficiencyRollService = {
  uuid: string;
  service_no: string;
  service_date: string;
  dealer_name: string;
  actual_meters: string;
  expected_meters: string | null;
  waste_ratio: string | null;
};

export type EfficiencyRollDetail = EfficiencyRollRow & {
  services: EfficiencyRollService[];
};

export type EfficiencySettings = {
  warning_waste_ratio: string;
};

export type ExpectationSource = "manual" | "network";

export type PartConsumptionExpectation = {
  uuid: string;
  product_uuid: string | null;
  category_uuid: string | null;
  product_name: string | null;
  category_name: string | null;
  body_type: string | null;
  part_key: string;
  expected_meters: string;
  source: ExpectationSource;
  sample_size: number;
};

export type ExpectationInput = {
  product_uuid?: string | null;
  category_uuid?: string | null;
  body_type?: string | null;
  part_key: string;
  expected_meters: string;
};

/** Period of the analytics endpoints (`YYYY-MM-DD`, inclusive). */
export type EfficiencyPeriod = {
  period_from: string;
  period_to: string;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export const EFFICIENCY_SUMMARY_EXPORT_PATH = "/v1/efficiency/summary/export";
export const EFFICIENCY_ROLLS_EXPORT_PATH = "/v1/efficiency/rolls/export";
export const EXPECTATIONS_IMPORT_PATHS = {
  upload: "/v1/platform/part-consumption-expectations/import",
  sample: "/v1/platform/part-consumption-expectations/import/sample",
};

const enc = encodeURIComponent;
const EXPECTATIONS = "/v1/platform/part-consumption-expectations";

export const efficiencyService = {
  summary(dimension: EfficiencyDimension, params: ServerListQuery) {
    return platformRequest<Page<EfficiencySummaryRow>>(
      "GET",
      "/v1/efficiency/summary",
      { query: { ...params, dimension } },
    );
  },
  trend(period: EfficiencyPeriod) {
    return platformRequest<EfficiencyTrendRow[]>(
      "GET",
      "/v1/efficiency/trend",
      { query: period },
    );
  },
  compare(period: EfficiencyPeriod) {
    return platformRequest<EfficiencyCompareRow[]>(
      "GET",
      "/v1/efficiency/compare",
      { query: period },
    );
  },
  settings() {
    return platformRequest<EfficiencySettings>(
      "GET",
      "/v1/efficiency/settings",
    );
  },
  rolls(params: ServerListQuery) {
    return platformRequest<Page<EfficiencyRollRow>>(
      "GET",
      "/v1/efficiency/rolls",
      { query: params },
    );
  },
  roll(uuid: string) {
    return platformRequest<EfficiencyRollDetail>(
      "GET",
      `/v1/efficiency/rolls/${enc(uuid)}`,
    );
  },
  expectations(params: ServerListQuery) {
    return platformRequest<Page<PartConsumptionExpectation>>(
      "GET",
      EXPECTATIONS,
      { query: params },
    );
  },
  createExpectation(input: ExpectationInput) {
    return platformRequest<PartConsumptionExpectation>("POST", EXPECTATIONS, {
      body: input,
    });
  },
  updateExpectation(uuid: string, input: ExpectationInput) {
    return platformRequest<PartConsumptionExpectation>(
      "PUT",
      `${EXPECTATIONS}/${enc(uuid)}`,
      { body: input },
    );
  },
  deleteExpectation(uuid: string) {
    return platformRequest<void>("DELETE", `${EXPECTATIONS}/${enc(uuid)}`);
  },
};

export const efficiencyKeys = {
  all: ["efficiency"] as const,
  settings: ["efficiency", "settings"] as const,
  summary: (dimension: EfficiencyDimension, params: ServerListQuery) =>
    ["efficiency", "summary", dimension, params] as const,
  trend: (period: EfficiencyPeriod) => ["efficiency", "trend", period] as const,
  compare: (period: EfficiencyPeriod) =>
    ["efficiency", "compare", period] as const,
  rolls: (params: ServerListQuery) => ["efficiency", "rolls", params] as const,
  roll: (uuid: string) => ["efficiency", "roll", uuid] as const,
  expectations: (params: ServerListQuery) =>
    ["efficiency", "expectations", params] as const,
};
