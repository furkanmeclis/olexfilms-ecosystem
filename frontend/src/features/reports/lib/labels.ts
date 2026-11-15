import type { ReportCategory } from "@/features/reports/lib/catalog";

/** i18n keys of the categories (literal keys for the i18n linter). */
export const CATEGORY_LABEL_KEYS: Record<ReportCategory, string> = {
  general: "reports.category.general",
  services: "reports.category.services",
  orders: "reports.category.orders",
  network: "reports.category.network",
  customers: "reports.category.customers",
  warranties: "reports.category.warranties",
  measurements: "reports.category.measurements",
  warehouse: "reports.category.warehouse",
  accounting: "reports.category.accounting",
  performance: "reports.category.performance",
  other: "reports.category.other",
};

/** Kind badge of a catalog entry. */
export const KIND_LABEL_KEYS: Record<string, string> = {
  summary: "reports.kind.summary",
  timeseries: "reports.kind.timeseries",
  distribution: "reports.kind.distribution",
  ranking: "reports.kind.ranking",
  table: "reports.kind.table",
};
