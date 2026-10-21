import type {
  StockTransfer,
  StockTransferCreateInput,
  StockTransferKind,
  StockTransferStatus,
  TransferDirection,
} from "@/features/transfers/services/transfers.service";
import type { StockUnitRow } from "@/features/stock/services/stock.service";
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

/** One scanned or picked unit of the request form. */
export type TransferLine = {
  barcode: string;
  /** Fixed barcodes only; empty moves the whole unit. */
  quantity: string;
  /**
   * Set for a unit picked from the stock list (TEC-870): true for a fixed
   * barcode (a quantity of 1..max is required), false for a serial piece
   * or roll (it moves whole, no quantity). Unset for a scanned barcode.
   */
  fixed?: boolean;
  /** Quantity on hand of a picked fixed barcode. */
  max?: number;
  /** Product name of a picked unit. */
  label?: string;
  /** Remaining meters of a picked roll. */
  meters?: string | null;
};

/** The stock row fields a picked line is built from. */
export type PickableUnit = Pick<
  StockUnitRow,
  "barcode" | "unit_kind" | "quantity" | "remaining_meters"
> & { product: Pick<StockUnitRow["product"], "name"> };

/**
 * Line of a unit picked from the stock list (TEC-870): a fixed barcode
 * starts at quantity 1 and is capped at its quantity on hand; a serial
 * piece or roll moves whole.
 */
export function lineFromUnit(u: PickableUnit): TransferLine {
  const fixed = u.unit_kind === "fixed";
  return {
    barcode: u.barcode,
    quantity: fixed ? "1" : "",
    fixed,
    ...(fixed ? { max: u.quantity } : { meters: u.remaining_meters }),
    label: u.product.name,
  };
}

/**
 * Picks or unpicks a stock unit: a barcode already on the list (picked or
 * scanned) is removed, any other is added.
 */
export function toggleUnitLine(
  lines: TransferLine[],
  u: PickableUnit,
): TransferLine[] {
  const barcode = normalizeBarcode(u.barcode);
  if (barcode === "") return lines;
  if (lines.some((l) => l.barcode === barcode)) {
    return lines.filter((l) => l.barcode !== barcode);
  }
  return [...lines, lineFromUnit({ ...u, barcode })];
}

/**
 * Quantity check of one line: a picked serial unit takes no quantity, a
 * picked fixed barcode needs 1..max, a scanned line keeps validQuantity.
 */
export function lineQuantityValid(l: TransferLine): boolean {
  if (l.fixed === false) return l.quantity.trim() === "";
  if (l.fixed === true) {
    const v = l.quantity.trim();
    if (v === "" || !validQuantity(v)) return false;
    return l.max === undefined || Number(v) <= l.max;
  }
  return validQuantity(l.quantity);
}

/** True when a picked fixed barcode asks for more than it holds. */
export function lineExceedsMax(l: TransferLine): boolean {
  const v = l.quantity.trim();
  return (
    l.fixed === true &&
    l.max !== undefined &&
    validQuantity(v) &&
    v !== "" &&
    Number(v) > l.max
  );
}

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
  if (!lines.every(lineQuantityValid)) return null;
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
