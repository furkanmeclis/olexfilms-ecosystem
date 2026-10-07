import type {
  AgingBuckets,
  CariAgingReport,
  MarginProduct,
  PnlLine,
  ReportKind,
  ReportQuery,
  StaffCostLine,
} from "@/features/staff-reports/services/staff-reports.service";

/** Aging buckets in column order with their i18n header keys. */
export const AGING_BUCKETS = [
  { key: "days_0_30", labelKey: "staff_reports.reports.aging.days_0_30" },
  { key: "days_31_60", labelKey: "staff_reports.reports.aging.days_31_60" },
  { key: "days_61_90", labelKey: "staff_reports.reports.aging.days_61_90" },
  { key: "days_90_plus", labelKey: "staff_reports.reports.aging.days_90_plus" },
] as const satisfies readonly { key: keyof AgingBuckets; labelKey: string }[];

export type AgingBucketKey = (typeof AGING_BUCKETS)[number]["key"];

/** The filter fields a report reads (the rest stay hidden). */
export function reportFields(kind: ReportKind) {
  return {
    period: kind !== "cari-aging",
    asOf: kind === "cari-aging",
    group: kind === "pnl",
  };
}

/** A from / to pair is valid when either side is empty or from ≤ to. */
export function isReportQueryValid(query: ReportQuery): boolean {
  return !query.from || !query.to || query.from <= query.to;
}

/** P&L chart rows (numbers; amounts arrive as decimal strings). */
export function pnlChartRows(lines: readonly PnlLine[]) {
  return lines.map((l) => ({
    label: l.label,
    income: Number(l.income),
    expense: Number(l.expense),
    net: Number(l.net),
  }));
}

/** Top products by revenue for the margin chart. */
export function marginChartRows(products: readonly MarginProduct[], top = 10) {
  return [...products]
    .sort((a, b) => Number(b.revenue) - Number(a.revenue))
    .slice(0, top)
    .map((p) => ({
      label: p.name,
      revenue: Number(p.revenue),
      gross_profit: p.gross_profit == null ? 0 : Number(p.gross_profit),
    }));
}

/** Receivable vs payable per aging bucket for the stacked chart. */
export function agingChartRows(
  report: Pick<CariAgingReport, "totals">,
  label: (key: AgingBucketKey) => string,
) {
  return AGING_BUCKETS.map(({ key }) => ({
    label: label(key),
    receivable: Number(report.totals.receivable.buckets[key]),
    payable: Math.abs(Number(report.totals.payable.buckets[key])),
  }));
}

/** Salary / advance / bonus per staff card for the stacked chart. */
export function staffCostChartRows(lines: readonly StaffCostLine[]) {
  return lines.map((l) => ({
    label: l.name,
    salary: Number(l.salary),
    advance: Number(l.advance),
    bonus: Number(l.bonus),
  }));
}
