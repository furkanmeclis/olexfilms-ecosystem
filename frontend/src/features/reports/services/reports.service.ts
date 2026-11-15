import type { components } from "@/generated/api";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type ReportCatalogItem = Schemas["ReportCatalogItem"];
export type ReportEnvelope = Schemas["ReportEnvelope"];

/**
 * TEC-1212: query of one report (TEC-1210 contract): a preset period or a
 * custom range, the bucket size and the comparison window. Undefined keys
 * are dropped from the URL.
 */
export type ReportQuery = {
  period?: string;
  range_from?: string;
  range_to?: string;
  granularity?: string;
  compare?: "none" | "previous" | "year";
  limit?: number;
  all_time?: boolean;
};

function toQuery(q: ReportQuery): Record<string, string | undefined> {
  return {
    period: q.range_from ? undefined : q.period,
    range_from: q.range_from,
    range_to: q.range_to,
    granularity: q.granularity,
    compare: q.compare && q.compare !== "none" ? q.compare : undefined,
    limit: q.limit === undefined ? undefined : String(q.limit),
    all_time: q.all_time ? "true" : undefined,
  };
}

const enc = encodeURIComponent;

export const reportsService = {
  catalog() {
    return platformRequest<{ items: ReportCatalogItem[] }>(
      "GET",
      "/v1/reports/catalog",
    );
  },
  report(key: string, query: ReportQuery) {
    return platformRequest<ReportEnvelope>("GET", `/v1/reports/${enc(key)}`, {
      query: toQuery(query),
    });
  },
  /** Server export (TEC-1210): same query, xlsx or csv. */
  async exportReport(
    key: string,
    query: ReportQuery,
    format: "xlsx" | "csv",
  ): Promise<void> {
    const { blob, filename } = await platformDownloadFile(
      `/v1/reports/${enc(key)}/export`,
      { ...toQuery(query), format },
    );
    triggerBrowserDownload(blob, filename ?? `${key}.${format}`);
  },
};

export const reportsKeys = {
  all: ["reports"] as const,
  catalog: (org: string) => ["reports", "catalog", org] as const,
  report: (org: string, key: string, query: ReportQuery) =>
    ["reports", "report", org, key, query] as const,
};
