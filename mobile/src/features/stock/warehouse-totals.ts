/**
 * Product totals of one warehouse (TEC-1069, TEC-1220). The rows come from
 * GET /v1/stock/warehouses/{uuid}/products (TEC-1090): the backend sums the
 * warehouse's locations, so a fixed barcode holding many pieces counts its
 * quantity, not 1, and nothing is capped at a number of units. These helpers
 * only shape the rows for the list. Pure, no `@/` imports, so node --test runs it.
 */

/** Page size and the most pages read for one warehouse (the API caps `limit` at 100). */
export const WAREHOUSE_TOTALS_PAGE = 100;
export const WAREHOUSE_TOTALS_MAX_PAGES = 50;

type WarehouseProductLike = {
  product: { uuid: string; sku: string; name: string; unit_type: string; category?: { name: string } | null };
  quantity: number;
  meters: string;
};

export type WarehouseProductTotal = {
  product: { uuid: string; sku: string; name: string };
  category?: string;
  /** Pieces, rolls and fixed barcode quantities in the warehouse (the API's `quantity`). */
  units: number;
  /** Meters left on the rolls; null for products not sold by the meter. */
  meters: number | null;
};

/**
 * Whether the warehouse has more product rows than the pages read
 * (WAREHOUSE_TOTALS_MAX_PAGES × WAREHOUSE_TOTALS_PAGE): the list and its
 * "out of stock" merge are then partial (TEC-1224).
 */
export function totalsTruncated(read: number, total: number | undefined): boolean {
  return total !== undefined && read < total;
}

/** Maps the API rows to list rows, keeping the API's order (by product name). */
export function warehouseProductTotals(rows: readonly WarehouseProductLike[]): WarehouseProductTotal[] {
  return rows.map((r) => {
    const m = Number(r.meters);
    return {
      product: { uuid: r.product.uuid, sku: r.product.sku, name: r.product.name },
      category: r.product.category?.name,
      units: r.quantity,
      meters: r.product.unit_type === 'roll_meter' ? (Number.isFinite(m) ? m : 0) : null,
    };
  });
}

/** Rows that hold stock in the warehouse (the "in stock" filter). */
export function inStockTotals(rows: readonly WarehouseProductTotal[]): WarehouseProductTotal[] {
  return rows.filter((r) => r.units > 0 || (r.meters ?? 0) > 0);
}

/** Rows matching a search (product name or SKU, case-insensitive). */
export function filterTotals(rows: readonly WarehouseProductTotal[], q: string): WarehouseProductTotal[] {
  const s = q.trim().toLocaleLowerCase();
  if (!s) return [...rows];
  return rows.filter((r) => r.product.name.toLocaleLowerCase().includes(s) || r.product.sku.toLocaleLowerCase().includes(s));
}

