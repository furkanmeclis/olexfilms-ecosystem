import { routes } from "@/config/routes";
import type {
  StockMovementDocument,
  StockReclassification,
  StockReclassificationStatus,
} from "@/features/stock/services/stock-ops.service";

/** Request statuses in the list filter (TEC-157). */
export const RECLASS_STATUSES = [
  "pending",
  "approved",
  "rejected",
  "cancelled",
] as const satisfies readonly StockReclassificationStatus[];

export function reclassStatusTone(
  status: StockReclassificationStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "pending":
      return "warning";
    case "approved":
      return "success";
    case "rejected":
      return "danger";
    default:
      return "default";
  }
}

/** What a unit operation needs to know about the unit. */
export type OperableUnit = {
  barcode: string;
  status: string;
  remaining_meters: string | null;
  product: {
    uuid: string;
    name: string;
    sku: string;
    unit_type: string;
    uses_fixed_barcode: boolean;
  } | null;
};

/** Units on hand (the backend refuses the rest with 409). */
function onHand(status: string): boolean {
  return status === "available" || status === "placed";
}

/**
 * A reclassification can be requested for a unit on hand that is not a
 * fixed barcode (422 STOCK_RECLASSIFICATION_FIXED_BARCODE).
 */
export function canRequestReclassification(unit: OperableUnit): boolean {
  return (
    onHand(unit.status) &&
    unit.product !== null &&
    !unit.product.uses_fixed_barcode
  );
}

/** Only a roll with meters left on hand can be split (TEC-184). */
export function canSplitRoll(unit: OperableUnit): boolean {
  return (
    onHand(unit.status) &&
    unit.product?.unit_type === "roll_meter" &&
    toCents(unit.remaining_meters ?? "") > 0
  );
}

/** Meters as the backend accepts them: up to 8 digits, 2 decimals. */
export const METERS_PATTERN = /^[0-9]{1,8}(\.[0-9]{1,2})?$/;

/** "12.5" → 1250; anything else → NaN. */
export function toCents(value: string): number {
  const v = value.trim().replace(",", ".");
  if (!METERS_PATTERN.test(v)) return Number.NaN;
  const [whole, frac = ""] = v.split(".");
  return Number(whole) * 100 + Number(frac.padEnd(2, "0"));
}

function fromCents(cents: number): string {
  return (cents / 100).toFixed(2);
}

export type SplitPreview =
  | { error: "required" | "format" | "positive" | "exceeds"; left: null }
  | { error: null; meters: string; left: string };

/**
 * Validates the meters to cut off a roll and previews what stays on it:
 * required, the backend format, more than 0 and at most the remaining
 * meters (422 STOCK_SPLIT_INSUFFICIENT_METERS otherwise).
 */
export function splitPreview(remaining: string, input: string): SplitPreview {
  if (input.trim() === "") return { error: "required", left: null };
  const cut = toCents(input);
  if (Number.isNaN(cut)) return { error: "format", left: null };
  if (cut <= 0) return { error: "positive", left: null };
  const have = toCents(remaining);
  if (Number.isNaN(have) || cut > have) {
    return { error: "exceeds", left: null };
  }
  return { error: null, meters: fromCents(cut), left: fromCents(have - cut) };
}

export type ReclassFormErrors = { target?: string; reason?: string };

/** Target product and a reason are required (reason ≤ 2000). */
export function reclassFormErrors(form: {
  target: string;
  reason: string;
}): ReclassFormErrors {
  const errors: ReclassFormErrors = {};
  if (!form.target) errors.target = "stock.reclass.errors.target_required";
  const reason = form.reason.trim();
  if (!reason) errors.reason = "stock.reclass.errors.reason_required";
  else if (reason.length > 2000) {
    errors.reason = "stock.reclass.errors.reason_too_long";
  }
  return errors;
}

export type ReclassActions = {
  approve: boolean;
  reject: boolean;
  cancel: boolean;
  /** The viewer requested it and could otherwise decide: show why not. */
  selfRequested: boolean;
};

/**
 * Which decisions the viewer gets on a request: approve and reject need
 * stock.reclassify (the requester cannot approve its own request, 403
 * STOCK_RECLASSIFICATION_SELF_APPROVAL); cancel needs stock.write in the
 * requesting organization. Only pending requests are decided.
 */
export function reclassActions(
  row: Pick<StockReclassification, "status" | "requested_by" | "organization">,
  viewer: {
    userUuid: string | null;
    organizationUuid: string | null;
    canReclassify: boolean;
    canWrite: boolean;
  },
): ReclassActions {
  const pending = row.status === "pending";
  const self =
    viewer.userUuid !== null && row.requested_by?.uuid === viewer.userUuid;
  return {
    approve: pending && viewer.canReclassify && !self,
    reject: pending && viewer.canReclassify,
    cancel:
      pending &&
      viewer.canWrite &&
      viewer.organizationUuid !== null &&
      row.organization.uuid === viewer.organizationUuid,
    selfRequested: pending && viewer.canReclassify && self,
  };
}

/** The tenant page of a movement's document, when there is one. */
export function movementDocumentHref(
  slug: string,
  doc: StockMovementDocument,
): string | null {
  switch (doc.type) {
    case "stock_entry":
      return routes.tenant.warehouse.entry(slug, doc.uuid);
    case "warehouse_transfer":
      return routes.tenant.warehouse.transfer(slug, doc.uuid);
    case "stock_count":
      return routes.tenant.warehouse.count(slug, doc.uuid);
    case "service":
      return routes.tenant.services.detail(slug, doc.uuid);
    case "order":
      return routes.tenant.orders.detail(slug, doc.uuid);
    case "transfer_request":
      return routes.tenant.transfers.detail(slug, doc.uuid);
    case "stock_reclassification":
      return routes.tenant.stock.reclassification(slug, doc.uuid);
    default:
      return null;
  }
}

/** A fresh idempotency key for one split attempt. */
export function newSplitKey(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return `split:${crypto.randomUUID()}`;
  }
  return `split:${Date.now()}:${Math.random().toString(36).slice(2)}`;
}
