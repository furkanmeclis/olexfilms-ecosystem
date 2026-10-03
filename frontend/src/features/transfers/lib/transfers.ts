import type {
  StockTransfer,
  StockTransferCreateInput,
  StockTransferKind,
  StockTransferStatus,
  TransferDirection,
} from "@/features/transfers/services/transfers.service";
import { isApiError } from "@/lib/api";

type T = (key: string, params?: Record<string, string | number>) => string;

/** Every request status in flow order (StockTransferStatus). */
export const TRANSFER_STATUSES: StockTransferStatus[] = [
  "requested",
  "approved",
  "rejected",
  "shipped",
  "received",
  "cancelled",
];

/** Request kinds (TEC-223): sibling transfers and returns to the parent. */
export const TRANSFER_KINDS: StockTransferKind[] = ["sibling", "return"];

export const TRANSFER_DIRECTIONS: TransferDirection[] = [
  "outgoing",
  "incoming",
  "approval",
];

/** Status chip tone. */
export function transferStatusTone(
  status: StockTransferStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "received":
      return "success";
    case "approved":
    case "shipped":
      return "warning";
    case "rejected":
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}

/**
 * Transitions that ask for a reason (kept as the decision note or the
 * cancel reason); the reason itself stays optional.
 */
export function transitionAsksReason(status: StockTransferStatus): boolean {
  return status === "rejected" || status === "cancelled";
}

/** Destructive transitions are shown as red buttons. */
export function transitionIsDestructive(status: StockTransferStatus): boolean {
  return status === "rejected" || status === "cancelled";
}

/**
 * Buttons of the detail page: the server's available_transitions in flow
 * order (the API is the only source of what the caller may do).
 */
export function transitionButtons(t: StockTransfer): StockTransferStatus[] {
  const allowed = new Set(t.available_transitions);
  return TRANSFER_STATUSES.filter((s) => allowed.has(s));
}

/** One scanned unit of the request form. */
export type TransferLine = {
  barcode: string;
  /** Fixed barcodes only; empty moves the whole unit. */
  quantity: string;
};

/** Trimmed barcode, or "" for blank input. */
export function normalizeBarcode(raw: string): string {
  return raw.trim();
}

/**
 * Adds a scanned barcode to the lines; a barcode already on the list is
 * ignored (each unit may appear once).
 */
export function addLine(lines: TransferLine[], raw: string): TransferLine[] {
  const barcode = normalizeBarcode(raw);
  if (barcode === "" || lines.some((l) => l.barcode === barcode)) return lines;
  return [...lines, { barcode, quantity: "" }];
}

/** A quantity field is valid when empty or a positive integer. */
export function validQuantity(raw: string): boolean {
  const v = raw.trim();
  return v === "" || /^[1-9][0-9]{0,6}$/.test(v);
}

/**
 * Request body of the form; null while the form is incomplete. A return
 * (TEC-223) carries kind=return and its parent as to_org_uuid.
 */
export function buildCreateBody(
  toOrgUuid: string,
  lines: TransferLine[],
  note: string,
  kind: StockTransferKind = "sibling",
): StockTransferCreateInput | null {
  if (toOrgUuid === "" || lines.length === 0) return null;
  if (!lines.every((l) => validQuantity(l.quantity))) return null;
  const body: StockTransferCreateInput = {
    ...(kind === "return" ? { kind } : {}),
    to_org_uuid: toOrgUuid,
    items: lines.map((l) =>
      l.quantity.trim() === ""
        ? { barcode: l.barcode }
        : { barcode: l.barcode, quantity: Number(l.quantity.trim()) },
    ),
  };
  const n = note.trim();
  if (n !== "") body.note = n;
  return body;
}

/** Index of the line an `items[3].barcode` error field points at, or -1. */
export function lineIndexOfField(field: string | undefined): number {
  const m = /^items\[(\d+)\]/.exec(field ?? "");
  return m ? Number(m[1]) : -1;
}

/**
 * Message for a failed transfer call: the detail code's text
 * (transfers.errors.<CODE>), then the error code's, then the server
 * message or the fallback.
 */
export function transferErrorMessage(
  err: unknown,
  t: T,
  fallback: string,
): string {
  if (!isApiError(err)) return fallback;
  for (const d of err.details ?? []) {
    if (!d.code) continue;
    const key = `transfers.errors.${d.code}`;
    const text = t(key);
    if (text !== key) return text;
  }
  const key = `transfers.errors.${err.code}`;
  const byCode = t(key);
  if (byCode !== key) return byCode;
  return fallback;
}

/** Number of pages for a total (at least one). */
export function pageCount(total: number, size: number): number {
  return Math.max(1, Math.ceil(total / size));
}
