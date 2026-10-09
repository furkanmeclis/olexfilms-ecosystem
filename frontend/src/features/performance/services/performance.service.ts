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
  reportCatalog: ["reports", "catalog"] as const,
  reportLayout: ["reports", "layout"] as const,
  report: (key: string, period: string | null) =>
    ["reports", "report", key, period] as const,
};
