/**
 * Product totals of one warehouse (TEC-1069). The API has no "products of a
 * warehouse" list, only the units that sit in it
 * (GET /v1/stock/warehouses/{uuid}/units?state=in), so the app sums those
 * units by product. Pure, no `@/` imports, so node --test runs it.
 */

/** Pages of 100 units read at most for one warehouse (the API caps `limit` at 100). */
export const WAREHOUSE_TOTALS_PAGE = 100;
export const WAREHOUSE_TOTALS_MAX_PAGES = 10;

type UnitLike = {
  product: { uuid: string; sku: string; name: string };
  remaining_meters: string | null;
};

export type WarehouseProductTotal = {
  product: { uuid: string; sku: string; name: string };
  /** Units (pieces or rolls) in the warehouse. */
  units: number;
  /** Meters left on the rolls; null when the product has no roll units. */
  meters: number | null;
};

/** Sums units by product, ordered by product name (then uuid, stable). */
export function warehouseProductTotals(units: readonly UnitLike[]): WarehouseProductTotal[] {
  const by = new Map<string, WarehouseProductTotal>();
  for (const u of units) {
    let row = by.get(u.product.uuid);
    if (!row) {
      row = { product: { uuid: u.product.uuid, sku: u.product.sku, name: u.product.name }, units: 0, meters: null };
      by.set(u.product.uuid, row);
    }
    row.units += 1;
    if (u.remaining_meters !== null && u.remaining_meters !== '') {
      const m = Number(u.remaining_meters);
      if (Number.isFinite(m)) row.meters = Math.round(((row.meters ?? 0) + m) * 100) / 100;
    }
  }
  return [...by.values()].sort(
    (a, b) => a.product.name.localeCompare(b.product.name) || (a.product.uuid < b.product.uuid ? -1 : a.product.uuid > b.product.uuid ? 1 : 0),
  );
}

/** Rows matching a search (product name or SKU, case-insensitive). */
export function filterTotals(rows: readonly WarehouseProductTotal[], q: string): WarehouseProductTotal[] {
  const s = q.trim().toLocaleLowerCase();
  if (!s) return [...rows];
  return rows.filter((r) => r.product.name.toLocaleLowerCase().includes(s) || r.product.sku.toLocaleLowerCase().includes(s));
}

/** Whether more units exist than the pages read (the totals are then partial). */
export function totalsTruncated(read: number, total: number | undefined): boolean {
  return total !== undefined && read < total;
}
