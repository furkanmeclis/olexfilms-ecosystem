import type {
  StockProduct,
  StockUnitRow,
  StockUnitStatus,
} from "@/features/stock/services/stock.service";

/** The two tabs of the page: units on hand, units consumed in services. */
export type StockTab = "stock" | "consumed";

export const STOCK_TABS: StockTab[] = ["stock", "consumed"];

/** Statuses offered in the on-hand filter (the list default is both). */
export const STOCK_FILTER_STATUSES = [
  "available",
  "placed",
] as const satisfies readonly StockUnitStatus[];

/** Units consumed by a service end in `used` (consumption movements). */
export const CONSUMED_STATUS: StockUnitStatus = "used";

/**
 * The purchase price column is shown only when at least one row carries a
 * price: the API answers null when the viewer's pricing.purchase.read does
 * not reach the organization (K8) or no price is set.
 */
export function showPurchasePrice(rows: readonly StockUnitRow[]): boolean {
  return rows.some((r) => r.purchase_price !== null);
}

/** Status chip tone. */
export function unitStatusTone(
  status: StockUnitStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "available":
      return "success";
    case "placed":
    case "in_transit":
      return "warning";
    case "used":
    case "void":
      return "danger";
    default:
      return "default";
  }
}

export type StockSummary = {
  /** Products with stock on hand. */
  products: number;
  /** Pieces and fixed barcode quantities. */
  quantity: number;
  /** Remaining roll meters. */
  meters: number;
};

/** Totals of the product projection (TEC-224 widget). */
export function summarizeProducts(
  items: readonly StockProduct[],
): StockSummary {
  let quantity = 0;
  let meters = 0;
  for (const row of items) {
    quantity += row.quantity;
    meters += parseDecimal(row.meters);
  }
  return { products: items.length, quantity, meters: round2(meters) };
}

/** Decimal string from the API ("12.50") as a number; garbage counts 0. */
export function parseDecimal(value: string | null | undefined): number {
  if (!value) return 0;
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}
