import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type PerformanceMetricValue = Schemas["PerformanceMetricValue"];
export type PerformanceRankingRow = Schemas["PerformanceRankingRow"];
export type PerformanceBenchmark = Schemas["PerformanceBenchmark"];
export type PerformanceTarget = Schemas["PerformanceTarget"];
export type PerformanceTargetInput = Schemas["PerformanceTargetInput"];
export type PerformanceRegionMap = Schemas["PerformanceRegionMap"];
export type PerformanceRegionMapItem = Schemas["PerformanceRegionMapItem"];
export type PerformanceEmptyRegion = Schemas["PerformanceEmptyRegion"];
export type PerformanceDealerMap = Schemas["PerformanceDealerMap"];
export type PerformanceDealerPoint = Schemas["PerformanceDealerPoint"];
export type ReportCatalogItem = Schemas["ReportCatalogItem"];
export type ReportEnvelope = Schemas["ReportEnvelope"];
export type ReportLayout = Schemas["ReportLayout"];
export type ReportWidget = Schemas["ReportWidget"];
export type ReportLayoutInput = Schemas["ReportLayoutInput"];
export type PerformanceStaffTarget = Schemas["PerformanceStaffTarget"];
export type PerformanceStaffTargetInput =
  Schemas["PerformanceStaffTargetInput"];
export type PerformanceBonus = Schemas["PerformanceBonus"];
export type PerformanceBonusApprovalInput =
  Schemas["PerformanceBonusApprovalInput"];
export type PerformanceBonusRule = Schemas["PerformanceBonusRule"];
export type PerformanceBonusRuleInput = Schemas["PerformanceBonusRuleInput"];
export type PerformanceBonusSettings = Schemas["PerformanceBonusSettings"];
export type PerformanceRule = Schemas["PerformanceRule"];
export type PerformanceRuleInput = Schemas["PerformanceRuleInput"];
export type PerformanceMember = Schemas["PerformanceMember"];
/** Distributor / dealer a target can be set for (tenant organizations). */
export type TargetOrganization = {
  uuid: string;
  name: string;
  type: string;
  currency?: string | null;
  contract_valid_until?: string | null;
  parent?: { uuid: string } | null;
};

/** Current / previous month of one dashboard metric (backend MetricDelta). */
export type PerformanceMetricDelta = {
  current?: PerformanceMetricValue | null;
  previous?: PerformanceMetricValue | null;
  delta?: string | null;
  delta_pct?: string | null;
};

export type PerformanceDashboard = Omit<
  Schemas["PerformanceDashboard"],
  "metrics"
> & {
  metrics: Record<string, PerformanceMetricDelta>;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type MapQuery = {
  level: "country" | "province" | "district";
  period: string;
  metric: string;
  country?: string;
};

const enc = encodeURIComponent;

/**
 * Performance module (TEC-492 / TEC-494) and the report widgets of the
 * corporate dashboard (TEC-495) through the BFF with the active
 * organization.
 */
export const performanceService = {
  dashboard(period: string) {
    return platformRequest<PerformanceDashboard>(
      "GET",
      "/v1/performance/dashboard",
      { query: { period } },
    );
  },
  ranking(params: ServerListQuery) {
    return platformRequest<Page<PerformanceRankingRow>>(
      "GET",
      "/v1/performance/ranking",
      { query: params },
    );
  },
  benchmark(period: string) {
    return platformRequest<PerformanceBenchmark>(
      "GET",
      "/v1/performance/benchmark",
      { query: { period } },
    );
  },
  regionMap(query: MapQuery) {
    return platformRequest<PerformanceRegionMap>("GET", "/v1/performance/map", {
      query,
    });
  },
  dealerMap(query: Omit<MapQuery, "level">) {
    return platformRequest<PerformanceDealerMap>(
      "GET",
      "/v1/performance/map/dealers",
      { query },
    );
  },
  createTarget(body: PerformanceTargetInput) {
    return platformRequest<PerformanceTarget>(
      "POST",
      "/v1/performance/targets",
      { body },
    );
  },
  targets(params: ServerListQuery) {
    return platformRequest<Page<PerformanceTarget>>(
      "GET",
      "/v1/performance/targets",
      { query: params },
    );
  },
  updateTarget(uuid: string, body: PerformanceTargetInput) {
    return platformRequest<PerformanceTarget>(
      "PUT",
      `/v1/performance/targets/${enc(uuid)}`,
      { body },
    );
  },
  deleteTarget(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/performance/targets/${enc(uuid)}`,
    );
  },
  /** Distributors and dealers of the organizations.read scope. */
  async targetOrganizations(q: string) {
    const data = await platformRequest<{ items: TargetOrganization[] }>(
      "GET",
      "/v1/tenant/organizations",
      { query: { limit: 50, ...(q ? { q } : {}) } },
    );
    return data.items ?? [];
  },
  async members() {
    const data = await platformRequest<{ items: PerformanceMember[] }>(
      "GET",
      "/v1/performance/members",
    );
    return data.items ?? [];
  },
  async staffTargets(query: { period_from: string; period_to: string }) {
    const data = await platformRequest<{ items: PerformanceStaffTarget[] }>(
      "GET",
      "/v1/performance/staff-targets",
      { query },
    );
    return data.items ?? [];
  },
  upsertStaffTarget(body: PerformanceStaffTargetInput) {
    return platformRequest<PerformanceStaffTarget>(
      "PUT",
      "/v1/performance/staff-targets",
      { body },
    );
  },
  deleteStaffTarget(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/performance/staff-targets/${enc(uuid)}`,
    );
  },
  bonuses(params: ServerListQuery) {
    return platformRequest<Page<PerformanceBonus>>(
      "GET",
      "/v1/performance/bonuses",
      { query: params },
    );
  },
  approveBonus(uuid: string, body: PerformanceBonusApprovalInput) {
    return platformRequest<PerformanceBonus>(
      "POST",
      `/v1/performance/bonuses/${enc(uuid)}/approve`,
      { body },
    );
  },
  cancelBonus(uuid: string) {
    return platformRequest<PerformanceBonus>(
      "POST",
      `/v1/performance/bonuses/${enc(uuid)}/cancel`,
    );
  },
  async bonusRules() {
    const data = await platformRequest<{ items: PerformanceBonusRule[] }>(
      "GET",
      "/v1/performance/bonus-rules",
    );
    return data.items ?? [];
  },
  createBonusRule(body: PerformanceBonusRuleInput) {
    return platformRequest<PerformanceBonusRule>(
      "POST",
      "/v1/performance/bonus-rules",
      { body },
    );
  },
  updateBonusRule(uuid: string, body: PerformanceBonusRuleInput) {
    return platformRequest<PerformanceBonusRule>(
      "PUT",
      `/v1/performance/bonus-rules/${enc(uuid)}`,
      { body },
    );
  },
  deleteBonusRule(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/performance/bonus-rules/${enc(uuid)}`,
    );
  },
  bonusSettings() {
    return platformRequest<PerformanceBonusSettings>(
      "GET",
      "/v1/performance/bonus-settings",
    );
  },
  saveBonusSettings(body: PerformanceBonusSettings) {
    return platformRequest<PerformanceBonusSettings>(
      "PUT",
      "/v1/performance/bonus-settings",
      { body },
    );
  },
  async rules() {
    const data = await platformRequest<{ items: PerformanceRule[] }>(
      "GET",
      "/v1/performance/rules",
    );
    return data.items ?? [];
  },
  createRule(body: PerformanceRuleInput) {
    return platformRequest<PerformanceRule>("POST", "/v1/performance/rules", {
      body,
    });
  },
  updateRule(uuid: string, body: PerformanceRuleInput) {
    return platformRequest<PerformanceRule>(
      "PUT",
      `/v1/performance/rules/${enc(uuid)}`,
      { body },
    );
  },
  deleteRule(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/performance/rules/${enc(uuid)}`,
    );
  },
  reportCatalog() {
    return platformRequest<{ items: ReportCatalogItem[] }>(
      "GET",
      "/v1/reports/catalog",
    );
  },
  report(key: string, query: { period?: string | null }) {
    const params: Record<string, string> = {};
    if (query.period) params.period = query.period;
    return platformRequest<ReportEnvelope>("GET", `/v1/reports/${enc(key)}`, {
      query: params,
    });
  },
  reportLayout() {
    return platformRequest<ReportLayout>("GET", "/v1/reports/layout");
  },
  saveReportLayout(body: ReportLayoutInput) {
    return platformRequest<ReportLayout>("PUT", "/v1/reports/layout", {
      body,
    });
  },
};

export const performanceKeys = {
  all: ["performance"] as const,
  dashboard: (period: string) => ["performance", "dashboard", period] as const,
  ranking: (params: ServerListQuery) =>
    ["performance", "ranking", params] as const,
  distributors: (period: string) =>
    ["performance", "distributors", period] as const,
  benchmark: (period: string) => ["performance", "benchmark", period] as const,
  regionMap: (query: MapQuery) => ["performance", "map", query] as const,
  dealerMap: (query: Omit<MapQuery, "level">) =>
    ["performance", "map-dealers", query] as const,
  targets: (params: ServerListQuery) =>
    ["performance", "targets", params] as const,
  targetOrganizations: (q: string) =>
    ["performance", "target-organizations", q] as const,
  members: ["performance", "members"] as const,
  staffTargets: (from: string, to: string) =>
    ["performance", "staff-targets", from, to] as const,
  bonuses: (params: ServerListQuery) =>
    ["performance", "bonuses", params] as const,
  bonusRules: ["performance", "bonus-rules"] as const,
  bonusSettings: ["performance", "bonus-settings"] as const,
  rules: ["performance", "rules"] as const,
  reportCatalog: ["reports", "catalog"] as const,
  reportLayout: ["reports", "layout"] as const,
  report: (key: string, period: string | null) =>
    ["reports", "report", key, period] as const,
};
