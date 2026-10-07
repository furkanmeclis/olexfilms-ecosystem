import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type StaffProfile = Schemas["StaffProfile"];
export type StaffProfileCreateInput = Schemas["StaffProfileCreateInput"];
export type StaffProfilePatchInput = Schemas["StaffProfilePatchInput"];
export type StaffPayment = Schemas["StaffPayment"];
export type StaffPaymentType = Schemas["StaffPaymentType"];
export type StaffPaymentCreateInput = Schemas["StaffPaymentCreateInput"];
export type StaffPayrollResult = Schemas["StaffPayrollResult"];
export type PnlReport = Schemas["AccountingPnlReport"];
export type PnlLine = Schemas["AccountingPnlLine"];
export type PnlGroup = PnlReport["group"];
export type MarginReport = Schemas["AccountingMarginReport"];
export type MarginProduct = Schemas["AccountingMarginProduct"];
export type MarginFigures = Schemas["AccountingMarginFigures"];
export type CariAgingReport = Schemas["AccountingCariAgingReport"];
export type CariAgingLine = CariAgingReport["lines"][number];
export type AgingBuckets = Schemas["AccountingAgingBuckets"];
export type StaffCostReport = Schemas["AccountingStaffCostReport"];
export type StaffCostLine = StaffCostReport["lines"][number];
export type ReportExportInput = Schemas["AccountingReportExportInput"];
export type ReportExportFormat = ReportExportInput["format"];
export type ReportExportJob = Schemas["ExportJob"];

export const STAFF_PAYMENT_TYPES: StaffPaymentType[] = [
  "salary",
  "advance",
  "bonus",
];

/** The four report pages of the own book (TEC-346 endpoints). */
export const REPORT_KINDS = [
  "pnl",
  "margin",
  "cari-aging",
  "staff-cost",
] as const;
export type ReportKind = (typeof REPORT_KINDS)[number];

export const REPORT_EXPORT_FORMATS: ReportExportFormat[] = [
  "csv",
  "xlsx",
  "pdf",
];

/** A report period: calendar days (UTC, inclusive). */
export type ReportPeriod = { from?: string; to?: string };

/** Report query per kind (the export body mirrors it). */
export type ReportQuery = ReportPeriod & {
  as_of?: string;
  group?: PnlGroup;
};

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/** Largest page the API serves (apiquery.MaxLimit). */
const PAGE_LIMIT = 100;
/** Safety stop of the page loop (10 000 rows). */
const MAX_PAGES = 100;

/**
 * Reads every page of a limit/offset list: the staff and payment lists
 * have no server sort or search, so the DataTable works on the full array.
 */
async function listAll<T>(
  fetchPage: (limit: number, offset: number) => Promise<Page<T>>,
): Promise<{ items: T[]; total: number }> {
  const items: T[] = [];
  let total = 0;
  for (let page = 0; page < MAX_PAGES; page++) {
    const res = await fetchPage(PAGE_LIMIT, page * PAGE_LIMIT);
    items.push(...res.items);
    total = res.total;
    if (res.items.length < PAGE_LIMIT || items.length >= total) break;
  }
  return { items, total };
}

/** Only set fields go into the query / body. */
function compact<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(
    Object.entries(value).filter(([, v]) => v !== undefined && v !== ""),
  ) as Partial<T>;
}

const staffPath = (uuid: string) =>
  `/v1/staff-profiles/${encodeURIComponent(uuid)}`;

/**
 * Staff cards, salary / advance / bonus payments and the month-end payroll
 * (TEC-345), the payment history (TEC-349) and the own-book reports
 * (TEC-346). Every call works on the active organization's book.
 */
export const staffReportsService = {
  listStaff() {
    return listAll((limit, offset) =>
      platformRequest<Page<StaffProfile>>("GET", "/v1/staff-profiles", {
        query: { limit, offset },
      }),
    );
  },

  createStaff(body: StaffProfileCreateInput) {
    return platformRequest<StaffProfile>("POST", "/v1/staff-profiles", {
      body,
    });
  },

  updateStaff(uuid: string, body: StaffProfilePatchInput) {
    return platformRequest<StaffProfile>("PATCH", staffPath(uuid), { body });
  },

  listPayments(staffUuid: string) {
    return listAll((limit, offset) =>
      platformRequest<Page<StaffPayment>>(
        "GET",
        `${staffPath(staffUuid)}/payments`,
        { query: { limit, offset } },
      ),
    );
  },

  createPayment(staffUuid: string, body: StaffPaymentCreateInput) {
    return platformRequest<StaffPayment>(
      "POST",
      `${staffPath(staffUuid)}/payments`,
      { body },
    );
  },

  runPayroll(period: string) {
    return platformRequest<StaffPayrollResult>(
      "POST",
      "/v1/staff-payments/payroll",
      { query: { period } },
    );
  },

  pnl(query: ReportQuery & { locale?: string }) {
    return platformRequest<PnlReport>("GET", "/v1/accounting/reports/pnl", {
      query: compact({
        from: query.from,
        to: query.to,
        group: query.group,
        locale: query.locale,
      }),
    });
  },

  margin(query: ReportPeriod) {
    return platformRequest<MarginReport>(
      "GET",
      "/v1/accounting/reports/margin",
      { query: compact({ from: query.from, to: query.to }) },
    );
  },

  cariAging(query: { as_of?: string }) {
    return platformRequest<CariAgingReport>(
      "GET",
      "/v1/accounting/reports/cari-aging",
      { query: compact({ as_of: query.as_of }) },
    );
  },

  staffCost(query: ReportPeriod) {
    return platformRequest<StaffCostReport>(
      "GET",
      "/v1/accounting/reports/staff-cost",
      { query: compact({ from: query.from, to: query.to }) },
    );
  },

  /**
   * Queues a report export job (202) on POST
   * /v1/accounting/reports/{kind}/export; the job is polled and downloaded
   * through /v1/accounting/exports/{uuid} like the statement export.
   */
  exportReport(
    kind: ReportKind,
    format: ReportExportFormat,
    query: ReportQuery,
    locale?: string,
  ) {
    const fields: ReportQuery =
      kind === "cari-aging"
        ? { as_of: query.as_of }
        : kind === "pnl"
          ? { from: query.from, to: query.to, group: query.group }
          : { from: query.from, to: query.to };
    return platformRequest<ReportExportJob>(
      "POST",
      `/v1/accounting/reports/${kind}/export`,
      { body: { format, ...compact(fields), ...compact({ locale }) } },
    );
  },
};

export const staffReportsKeys = {
  all: (org: string) => ["staff-reports", org] as const,
  staff: (org: string) => ["staff-reports", org, "staff"] as const,
  payments: (org: string, staff: string) =>
    ["staff-reports", org, "payments", staff] as const,
  report: (org: string, kind: ReportKind, query: unknown) =>
    ["staff-reports", org, "report", kind, query] as const,
};
