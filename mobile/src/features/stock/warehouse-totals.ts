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

/** TEC-1229 balance split of a stock row (eldeki / rezerve / kullanılabilir / yolda). */
export type BalanceLike = {
  reserved_quantity: number;
  available_quantity: number;
  incoming_quantity: number;
  quarantine_quantity: number;
  available_meters: string;
  reserved_meters: string;
};

type WarehouseProductLike = {
  product: { uuid: string; sku: string; name: string; unit_type: string; category?: { name: string } | null };
  quantity: number;
  meters: string;
  balance?: BalanceLike | null;
};

export type WarehouseProductTotal = {
  product: { uuid: string; sku: string; name: string };
  category?: string;
  /** Pieces, rolls and fixed barcode quantities in the warehouse (the API's `quantity`). */
  units: number;
  /** Meters left on the rolls; null for products not sold by the meter. */
  meters: number | null;
  /** TEC-1229: reserved for orders, available (on hand - reserved - quarantine), incoming. */
  reserved: number;
  available: number;
  incoming: number;
};

/** Maps the API rows to list rows, keeping the API's order (by product name). */
export function warehouseProductTotals(rows: readonly WarehouseProductLike[]): WarehouseProductTotal[] {
  return rows.map((r) => {
    const m = Number(r.meters);
    return {
      product: { uuid: r.product.uuid, sku: r.product.sku, name: r.product.name },
      category: r.product.category?.name,
      units: r.quantity,
      meters: r.product.unit_type === 'roll_meter' ? (Number.isFinite(m) ? m : 0) : null,
      reserved: r.balance?.reserved_quantity ?? 0,
      available: r.balance?.available_quantity ?? r.quantity,
      incoming: r.balance?.incoming_quantity ?? 0,
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


/**
 * TEC-1229: the parts of the balance line under a stock row. Available is
 * always shown when something is reserved or incoming; zero parts are left
 * out. Keys are i18n keys of the warehouse namespace.
 */
export function balanceParts(b: { reserved: number; available: number; incoming: number }): { key: string; count: number }[] {
  if (b.reserved === 0 && b.incoming === 0) return [];
  const out = [{ key: 'm.balance_available', count: b.available }];
  if (b.reserved > 0) out.push({ key: 'm.balance_reserved', count: b.reserved });
  if (b.incoming > 0) out.push({ key: 'm.balance_incoming', count: b.incoming });
  return out;
}
