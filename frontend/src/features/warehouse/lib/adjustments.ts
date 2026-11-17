import { isApiError } from "@/lib/api";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";

/** TEC-1226 adjustment types, statuses and inspection choices. */
export const ADJUSTMENT_TYPES = [
  "waste",
  "damage",
  "scrap",
  "loss",
  "found",
  "other",
] as const;
export const ADJUSTMENT_STATUSES = [
  "draft",
  "pending_approval",
  "posted",
  "cancelled",
] as const;
export const ADJUSTMENT_SOURCES = ["manual", "return_inspection"] as const;
export const INSPECTION_DECISIONS = [
  "restock",
  "scrap",
  "supplier_return",
] as const;
export const RETURN_REASONS = [
  "defective",
  "damaged",
  "wrong_product",
  "excess",
  "customer_return",
  "other",
] as const;

export type InspectionDecision = (typeof INSPECTION_DECISIONS)[number];

export function adjustmentStatusTone(
  status: string,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "posted":
      return "success";
    case "pending_approval":
      return "warning";
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}

/** Only a draft takes lines, header edits and photo removals. */
export function isDraft(a: { status: string }): boolean {
  return a.status === "draft";
}

export function canCancelAdjustment(a: { status: string }): boolean {
  return a.status === "draft" || a.status === "pending_approval";
}

/** A damage document needs a photo before it is submitted. */
export function needsPhoto(a: {
  type: string;
  photos?: readonly unknown[];
}): boolean {
  return a.type === "damage" && (a.photos?.length ?? 0) === 0;
}

/**
 * Meters as typed by the user ("2,5" or "2.5"), normalized to the API
 * format with a dot; empty or invalid input is undefined.
 */
export function normalizeMeters(raw: string): string | undefined {
  const v = raw.trim().replace(",", ".");
  if (!v) return undefined;
  if (!/^\d+(\.\d{1,2})?$/.test(v)) return undefined;
  return Number(v) > 0 ? v : undefined;
}

type T = (key: string, params?: Record<string, string | number>) => string;

/** Translated error of the adjustment screens. */
export function adjustmentErrorMessage(
  err: unknown,
  t: T,
  fallback: string,
): string {
  if (isApiError(err)) {
    const key = `stock_adjustments.errors.${err.code}`;
    const text = t(key);
    if (text !== key) return text;
  }
  return warehouseErrorMessage(err, t, fallback);
}
