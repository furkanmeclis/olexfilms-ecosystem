import { exportsService } from "@/features/io/services/exports.service";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type FailureRateRow = Schemas["WarrantyClaimFailureRateRow"];
export type FailureRateReport = Schemas["WarrantyClaimFailureRateReport"];
export type ByDealerRow = Schemas["WarrantyClaimsByDealerRow"];
export type ByDealerReport = Schemas["WarrantyClaimsByDealerReport"];
export type PartsRow = Schemas["WarrantyClaimPartsReportRow"];
export type PartsReport = Schemas["WarrantyClaimPartsReport"];
export type ClaimReportExportRequest =
  Schemas["WarrantyClaimReportExportRequest"];
export type ClaimReportExportFormat = ClaimReportExportRequest["format"];

export type ClaimReportGroup = "product" | "lot";
/** Report families; each has its own read and export endpoint (TEC-338). */
export type ClaimReportKind = "failure-rate" | "by-dealer" | "parts";

/** Period filter shared by the three reports (YYYY-MM-DD, both optional). */
export type ClaimReportPeriod = { from?: string; to?: string };

const BASE = "/v1/warranty-claims/reports";

export function claimReportPath(kind: ClaimReportKind) {
  return `${BASE}/${kind}`;
}

export function claimReportExportPath(kind: ClaimReportKind) {
  return `${BASE}/${kind}/export`;
}

/**
 * Warranty claim reports (TEC-338) through the BFF. Every read and export
 * follows the warranty_claims.read scope of the active organization, so a
 * distributor only ever receives its own subtree.
 */
export const claimReportsService = {
  failureRate(query: ClaimReportPeriod & { group: ClaimReportGroup }) {
    return platformRequest<FailureRateReport>(
      "GET",
      claimReportPath("failure-rate"),
      { query },
    );
  },
  byDealer(query: ClaimReportPeriod) {
    return platformRequest<ByDealerReport>(
      "GET",
      claimReportPath("by-dealer"),
      {
        query,
      },
    );
  },
  parts(query: ClaimReportPeriod) {
    return platformRequest<PartsReport>("GET", claimReportPath("parts"), {
      query,
    });
  },
  /** Queues an organization export job (listed under Exports). */
  requestExport(kind: ClaimReportKind, body: ClaimReportExportRequest) {
    return exportsService.request(claimReportExportPath(kind), {
      format: body.format,
      locale: body.locale,
      query: body.query as Record<string, string> | undefined,
    });
  },
};

export const claimReportKeys = {
  all: ["warranty-claim-reports"] as const,
  failureRate: (query: ClaimReportPeriod & { group: ClaimReportGroup }) =>
    [...claimReportKeys.all, "failure-rate", query] as const,
  byDealer: (query: ClaimReportPeriod) =>
    [...claimReportKeys.all, "by-dealer", query] as const,
  parts: (query: ClaimReportPeriod) =>
    [...claimReportKeys.all, "parts", query] as const,
};
