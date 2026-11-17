import type { StockReorderRuleInput } from "@/features/warehouse/services/warehouse.service";

/** TEC-1229: stock ledger movement types (movement list filter). */
export const MOVEMENT_TYPES = [
  "entry",
  "placement",
  "transfer_out",
  "transfer_in",
  "transfer_cancel_restore",
  "order_out",
  "received",
  "order_cancel_restore",
  "consumption",
  "partial_consumption",
  "return",
  "sale",
  "reclassification",
  "count_adjustment",
  "void",
  "external_outbound",
  "split",
] as const;

/** TEC-1229: reorder rule levels, most urgent first. */
export const REORDER_LEVELS = [
  "below_min",
  "reorder",
  "days_low",
  "ok",
] as const;

export type ReorderLevel = (typeof REORDER_LEVELS)[number];

export function reorderLevelTone(
  level: string,
): "danger" | "warning" | "default" | "success" {
  switch (level) {
    case "below_min":
      return "danger";
    case "reorder":
      return "warning";
    case "days_low":
      return "default";
    default:
      return "success";
  }
}

/** Text fields of the reorder rule form. */
export type ReorderForm = {
  warehouse_uuid: string;
  product_uuid: string;
  min_quantity: string;
  reorder_point: string;
  max_quantity: string;
  warning_days: string;
  cover_days: string;
};

export type ReorderParse =
  | { ok: true; value: StockReorderRuleInput }
  | { ok: false; error: "required" | "number" | "order" | "days" | "empty" };

const num = (s: string): number | null | undefined => {
  const v = s.trim();
  if (v === "") return null;
  if (!/^\d+$/.test(v)) return undefined;
  return Number(v);
};

/**
 * Validates the form like the backend: integers, min <= reorder point <=
 * max, days 1..3650 and at least one threshold.
 */
export function reorderInput(f: ReorderForm): ReorderParse {
  if (!f.warehouse_uuid || !f.product_uuid) {
    return { ok: false, error: "required" };
  }
  const min = num(f.min_quantity);
  const point = num(f.reorder_point);
  const maxQ = num(f.max_quantity);
  const warn = num(f.warning_days);
  const cover = num(f.cover_days);
  if ([min, point, maxQ, warn, cover].some((v) => v === undefined)) {
    return { ok: false, error: "number" };
  }
  const m = min ?? 0;
  if (point != null && point < m) return { ok: false, error: "order" };
  if (maxQ != null && (maxQ < Math.max(m, point ?? 0) || maxQ === 0)) {
    return { ok: false, error: "order" };
  }
  for (const d of [warn, cover]) {
    if (d != null && (d < 1 || d > 3650)) return { ok: false, error: "days" };
  }
  if (m === 0 && point == null && maxQ == null && warn == null) {
    return { ok: false, error: "empty" };
  }
  return {
    ok: true,
    value: {
      warehouse_uuid: f.warehouse_uuid,
      product_uuid: f.product_uuid,
      min_quantity: m,
      reorder_point: point ?? null,
      max_quantity: maxQ ?? null,
      warning_days: warn ?? null,
      cover_days: cover ?? null,
    },
  };
}
