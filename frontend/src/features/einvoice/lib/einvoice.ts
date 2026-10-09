import type {
  Einvoice,
  EinvoiceProfile,
  EinvoiceSourceType,
  EinvoiceStatus,
  EinvoiceValidationStatus,
} from "@/features/einvoice/services/einvoice.service";

export const EINVOICE_STATUSES: EinvoiceStatus[] = [
  "draft",
  "failed",
  "archived",
  "voided",
];
export const EINVOICE_PROFILES: EinvoiceProfile[] = [
  "EARSIVFATURA",
  "TEMELFATURA",
  "TICARIFATURA",
];
export const EINVOICE_SOURCE_TYPES: EinvoiceSourceType[] = [
  "order",
  "service_subscription",
];

type Tone = "default" | "success" | "warning" | "danger";

export function statusTone(status: EinvoiceStatus): Tone {
  switch (status) {
    case "archived":
      return "success";
    case "failed":
      return "danger";
    case "voided":
      return "warning";
    default:
      return "default";
  }
}

export function validationTone(status: EinvoiceValidationStatus): Tone {
  if (status === "valid") return "success";
  return status === "invalid" ? "danger" : "warning";
}

/** Series: exactly 3 characters A-Z / 0-9; TMP is reserved for drafts. */
export const SERIES_PATTERN = /^[A-Z0-9]{3}$/;
export const RESERVED_SERIES = "TMP";

export type SeriesError = "length" | "chars" | "reserved" | null;

export function seriesError(value: string): SeriesError {
  const v = value.trim();
  if (v.length !== 3) return "length";
  if (!SERIES_PATTERN.test(v)) return "chars";
  if (v === RESERVED_SERIES) return "reserved";
  return null;
}

export const VOID_REASON_MAX = 1000;

/** A void needs a reason (1–1000 characters after trimming). */
export function validVoidReason(reason: string) {
  const r = reason.trim();
  return r.length > 0 && r.length <= VOID_REASON_MAX;
}

type Actionable = Pick<Einvoice, "status" | "validation_status">;

/**
 * Archive is open for a draft or a failed invoice whose last validation
 * was not `invalid` (rule warnings may be retried after a fix); an invalid
 * invoice is voided and drafted again.
 */
export function canArchive(invoice: Actionable) {
  return (
    (invoice.status === "draft" || invoice.status === "failed") &&
    invoice.validation_status !== "invalid"
  );
}

/** Void: a mark only, open until the invoice is voided. */
export function canVoid(invoice: Pick<Einvoice, "status">) {
  return invoice.status !== "voided";
}

export function hasFiles(invoice: Pick<Einvoice, "status" | "has_xml">) {
  return (
    (invoice.status === "archived" || invoice.status === "voided") &&
    invoice.has_xml
  );
}

/** PDF re-render: archived invoices (missing or failed PDF, new XSLT). */
export function canRetryPdf(invoice: Pick<Einvoice, "status">) {
  return invoice.status === "archived";
}

export type TimelineStep = {
  id: "created" | "failed" | "archived" | "voided";
  at: string;
  note?: string | null;
};

/**
 * Status history derived from the invoice row: drafted, failed validation
 * (last attempt), archived with its number, voided with the reason.
 */
export function statusTimeline(
  invoice: Pick<
    Einvoice,
    | "status"
    | "number"
    | "created_at"
    | "updated_at"
    | "issue_date"
    | "error"
    | "voided_at"
    | "void_reason"
  >,
): TimelineStep[] {
  const steps: TimelineStep[] = [{ id: "created", at: invoice.created_at }];
  if (invoice.status === "failed") {
    steps.push({ id: "failed", at: invoice.updated_at, note: invoice.error });
  }
  if (invoice.number) {
    steps.push({
      id: "archived",
      at: invoice.issue_date,
      note: invoice.number,
    });
  }
  if (invoice.status === "voided" && invoice.voided_at) {
    steps.push({
      id: "voided",
      at: invoice.voided_at,
      note: invoice.void_reason,
    });
  }
  return steps;
}

function csvCell(value: string) {
  return /[",\n;]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}

/** CSV of the invoice list (the list has no export job). */
export function invoicesCsv(rows: readonly Einvoice[]) {
  const header = [
    "number",
    "ettn",
    "buyer",
    "profile",
    "issue_date",
    "currency",
    "tax_exclusive",
    "tax_total",
    "payable",
    "status",
    "validation_status",
  ];
  const lines = rows.map((r) =>
    [
      r.number ?? "",
      r.uuid,
      r.buyer_organization?.name ?? r.buyer.name,
      r.profile,
      r.issue_date,
      r.currency,
      r.tax_exclusive,
      r.tax_total,
      r.payable,
      r.status,
      r.validation_status,
    ]
      .map((v) => csvCell(String(v)))
      .join(","),
  );
  return [header.join(","), ...lines].join("\n");
}

/** Decimal string → number for the currency formatter. */
export function amount(value: string) {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}
