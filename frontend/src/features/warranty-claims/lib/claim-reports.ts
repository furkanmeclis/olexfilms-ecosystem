import type {
  ByDealerRow,
  ClaimReportExportFormat,
  ClaimReportExportRequest,
  ClaimReportGroup,
  ClaimReportKind,
  ClaimReportPeriod,
  FailureRateRow,
} from "@/features/warranty-claims/services/claim-reports.service";
import { formatPercent, type FormatContext } from "@/lib/i18n/format";

/** Report tabs of the page; product and lot share the failure-rate report. */
export const CLAIM_REPORT_TABS = ["product", "lot", "dealer", "parts"] as const;
export type ClaimReportTab = (typeof CLAIM_REPORT_TABS)[number];

export const CLAIM_REPORT_EXPORT_FORMATS: ClaimReportExportFormat[] = [
  "xlsx",
  "csv",
];

/** Bars drawn in the failure-rate chart (highest rates first). */
export const CHART_BAR_LIMIT = 10;

export function tabReportKind(tab: ClaimReportTab): ClaimReportKind {
  if (tab === "dealer") return "by-dealer";
  if (tab === "parts") return "parts";
  return "failure-rate";
}

export function tabGroup(tab: ClaimReportTab): ClaimReportGroup | undefined {
  return tab === "product" || tab === "lot" ? tab : undefined;
}

/**
 * Rates come from the API as fractions (0.125 = 12.5 %). Formatted with
 * Intl percent in the user's locale ("12.5%", "%12,5", "12,5 %").
 */
export function formatClaimRate(
  rate: number | null | undefined,
  ctx: FormatContext,
): string {
  return formatPercent(rate, ctx, 1);
}

/** Drops empty bounds so the API applies its own open period. */
export function buildPeriodQuery(period: ClaimReportPeriod): ClaimReportPeriod {
  const out: ClaimReportPeriod = {};
  if (period.from) out.from = period.from;
  if (period.to) out.to = period.to;
  return out;
}

/** "from after to" is rejected before any request is sent. */
export function isPeriodValid(period: ClaimReportPeriod): boolean {
  return !period.from || !period.to || period.from <= period.to;
}

export function buildExportRequest(
  tab: ClaimReportTab,
  period: ClaimReportPeriod,
  format: ClaimReportExportFormat,
  locale: string,
): ClaimReportExportRequest {
  const group = tabGroup(tab);
  // `group` only applies to the failure-rate report; the generated type marks
  // it required because of its server-side default.
  const query = {
    ...buildPeriodQuery(period),
    ...(group ? { group } : {}),
  } as ClaimReportExportRequest["query"];
  return { format, locale, query };
}

/** Product label, or "product · lot" in the lot grouping. */
export function failureRowLabel(row: FailureRateRow): string {
  if (row.group === "lot") {
    return `${row.product_name} · ${row.lot_code ?? "—"}`;
  }
  return row.product_name;
}

export function failureRowKey(row: FailureRateRow): string {
  return `${row.product_uuid}:${row.lot_uuid ?? ""}`;
}

export type FailureRateBar = {
  key: string;
  label: string;
  /** Approved failure rate in percent points for the chart axis. */
  approvedPercent: number;
  claimPercent: number;
};

export function toFailureRateBars(
  rows: readonly FailureRateRow[],
  limit = CHART_BAR_LIMIT,
): FailureRateBar[] {
  return [...rows]
    .filter((r) => r.claim_count > 0)
    .sort(
      (a, b) =>
        b.approved_rate - a.approved_rate ||
        b.claim_rate - a.claim_rate ||
        b.claim_count - a.claim_count,
    )
    .slice(0, limit)
    .map((r) => ({
      key: failureRowKey(r),
      label: failureRowLabel(r),
      approvedPercent: round1(r.approved_rate * 100),
      claimPercent: round1(r.claim_rate * 100),
    }));
}

function round1(n: number) {
  return Math.round(n * 10) / 10;
}

export type DealerOption = { value: string; label: string };

/**
 * Dealer filter options. They come only from the by-dealer rows, which the
 * API already limits to the caller's warranty_claims.read scope: a
 * distributor sees its own subtree and nothing else (no global list).
 */
export function dealerFilterOptions(
  rows: readonly ByDealerRow[],
): DealerOption[] {
  const seen = new Set<string>();
  const out: DealerOption[] = [];
  for (const row of rows) {
    if (row.organization_type === "center") continue;
    if (seen.has(row.organization_uuid)) continue;
    seen.add(row.organization_uuid);
    out.push({ value: row.organization_uuid, label: row.organization_name });
  }
  return out.sort((a, b) => a.label.localeCompare(b.label));
}
