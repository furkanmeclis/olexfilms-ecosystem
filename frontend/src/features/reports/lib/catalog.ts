import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";

/**
 * TEC-1212: report categories of the reports center. The backend catalog
 * (GET /v1/reports/catalog) is already filtered by permission, module and
 * reach; the web only groups it. Order is the display order.
 */
export const REPORT_CATEGORIES = [
  "general",
  "services",
  "orders",
  "network",
  "customers",
  "warranties",
  "measurements",
  "warehouse",
  "accounting",
  "performance",
  "other",
] as const;

export type ReportCategory = (typeof REPORT_CATEGORIES)[number];

/** Reports whose key prefix does not tell the category. */
const EXPLICIT: Record<string, ReportCategory> = {
  overview: "general",
  "activities.recent": "general",
  "dealers.performance": "network",
  "dealers.top_by_warranty": "network",
  "services.by_org": "network",
  "orders.by_buyer": "network",
  "orders.inactive_buyers": "network",
  "orders.fulfillment": "warehouse",
};

/**
 * Snapshot reports: they show the state now and take no period, range,
 * compare or granularity (backend Definition.Periodic = false). Sending one
 * answers 400, so the shared period of an overview is dropped for them.
 */
export const SNAPSHOT_REPORTS: ReadonlySet<string> = new Set([
  "stock.summary",
  "activities.recent",
  "stock.aging",
]);

const PERIOD_PARAMS = [
  "period",
  "range_from",
  "range_to",
  "compare",
  "granularity",
] as const;

/** The query a report accepts: snapshot reports lose the period params. */
export function reportQueryFor<Q extends Record<string, unknown>>(
  key: string,
  query: Q,
): Q {
  if (!SNAPSHOT_REPORTS.has(key)) return query;
  const out: Record<string, unknown> = { ...query };
  for (const k of PERIOD_PARAMS) delete out[k];
  return out as Q;
}

const PREFIX: Record<string, ReportCategory> = {
  services: "services",
  orders: "orders",
  network: "network",
  dealers: "network",
  customers: "customers",
  warranties: "warranties",
  measurements: "measurements",
  stock: "warehouse",
  warehouse: "warehouse",
  accounting: "accounting",
  performance: "performance",
  efficiency: "performance",
};

/** Category of one backend report key. */
export function reportCategory(key: string): ReportCategory {
  const explicit = EXPLICIT[key];
  if (explicit) return explicit;
  const prefix = key.split(".")[0] ?? "";
  return PREFIX[prefix] ?? "other";
}

/** Groups items by category, in REPORT_CATEGORIES order; empty ones drop. */
export function groupByCategory<T>(
  items: readonly T[],
  categoryOf: (item: T) => ReportCategory,
): { category: ReportCategory; items: T[] }[] {
  const buckets = new Map<ReportCategory, T[]>();
  for (const item of items) {
    const c = categoryOf(item);
    const list = buckets.get(c);
    if (list) list.push(item);
    else buckets.set(c, [item]);
  }
  return REPORT_CATEGORIES.filter((c) => buckets.has(c)).map((c) => ({
    category: c,
    items: buckets.get(c) ?? [],
  }));
}

/**
 * Dedicated report pages that live outside the generic /v1/reports
 * contract (TEC-349 accounting, TEC-340 warranty claims, TEC-489
 * efficiency, TEC-496 performance). The reports center lists them in their
 * category with the same gates as their sidebar entries.
 */
export type SpecialReport = {
  key: string;
  category: ReportCategory;
  titleKey: string;
  descriptionKey: string;
  href: (slug: string) => string;
  permission: string;
  feature: string;
  orgTypes?: readonly string[];
};

export const SPECIAL_REPORTS: readonly SpecialReport[] = [
  {
    key: "accounting.reports",
    category: "accounting",
    titleKey: "reports.special.accounting.title",
    descriptionKey: "reports.special.accounting.description",
    href: (slug) => routes.tenant.accountingReports(slug),
    permission: permissions.accounting.read,
    feature: "accounting",
  },
  {
    key: "warranty_claims.reports",
    category: "warranties",
    titleKey: "reports.special.warranty_claims.title",
    descriptionKey: "reports.special.warranty_claims.description",
    href: (slug) => routes.tenant.warrantyClaims.reports(slug),
    permission: permissions.warrantyClaims.read,
    feature: "warranty_claims",
    orgTypes: ["center", "distributor"],
  },
  {
    key: "efficiency",
    category: "performance",
    titleKey: "reports.special.efficiency.title",
    descriptionKey: "reports.special.efficiency.description",
    href: (slug) => routes.tenant.efficiency.root(slug),
    permission: permissions.efficiency.read,
    feature: "efficiency",
  },
  {
    key: "performance",
    category: "performance",
    titleKey: "reports.special.performance.title",
    descriptionKey: "reports.special.performance.description",
    href: (slug) => routes.tenant.performance.root(slug),
    permission: permissions.performance.read,
    feature: "performance",
  },
];

/** Special reports open to the caller (features null: still unknown). */
export function visibleSpecialReports(
  can: (permission: string) => boolean,
  orgType: string | null | undefined,
  features: readonly string[] | null,
): SpecialReport[] {
  if (!features) return [];
  return SPECIAL_REPORTS.filter(
    (r) =>
      can(r.permission) &&
      features.includes(r.feature) &&
      (!r.orgTypes || (orgType != null && r.orgTypes.includes(orgType))),
  );
}

/**
 * Read permissions behind the reports center entry (TEC-1212): any report
 * domain is enough; the performance permission is no longer required.
 */
export const REPORTS_NAV_PERMISSIONS: string[] = [
  permissions.services.read,
  permissions.orders.read,
  permissions.customers.read,
  permissions.warranties.read,
  permissions.measurements.read,
  permissions.stock.read,
  permissions.warehouse.read,
  permissions.accounting.read,
  permissions.warrantyClaims.read,
  permissions.efficiency.read,
  permissions.performance.read,
  permissions.organizations.tenantRead,
];
