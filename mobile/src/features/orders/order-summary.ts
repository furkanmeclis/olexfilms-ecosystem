/**
 * Order card and timeline helpers (TEC-1055, TEC-1067, TEC-1059). Pure, no
 * `@/` imports, so node --test runs it directly.
 */

const STATUSES = [
  'draft',
  'submitted',
  'approved',
  'preparing',
  'ready',
  'processing',
  'shipped',
  'delivered',
  'received',
  'cancelling',
  'cancelled',
] as const;

/** Timeline actions that are not a status (OrderTimelineEntry.action, TEC-1060). */
const STEP_ACTIONS = ['created', 'unit_assigned', 'unit_unassigned', 'updated'] as const;

/**
 * Translation key of a timeline / last activity action: a status the order
 * moved to reads as that status (`status.<s>`), the other steps as
 * `step.<action>`. An unknown action (a newer API) gives null; the caller
 * then shows the raw value.
 */
export function actionKey(action: string): string | null {
  if ((STATUSES as readonly string[]).includes(action)) return `status.${action}`;
  if ((STEP_ACTIONS as readonly string[]).includes(action)) return `step.${action}`;
  return null;
}

type Amounts = { total_quantity?: number | null; total_meters?: string | null };

/**
 * What an order holds: the pieces and the roll meters. Each part is present
 * only when the order has it; an order with neither (no line yet) gives
 * both null.
 */
export function orderAmounts(o: Amounts): { pieces: number | null; meters: number | null } {
  const pieces = o.total_quantity && o.total_quantity > 0 ? o.total_quantity : null;
  const m = o.total_meters == null || o.total_meters === '' ? NaN : Number(o.total_meters);
  return { pieces, meters: Number.isFinite(m) && m > 0 ? m : null };
}

/**
 * The TRY figure shown small next to a foreign currency total: only when the
 * API gave one and the order is not already in TRY.
 */
export function liveTryAmount(o: { currency: string; live_total_try?: string | null }): string | null {
  if (o.currency === 'TRY') return null;
  const v = o.live_total_try;
  if (v == null || v === '' || !Number.isFinite(Number(v))) return null;
  return v;
}

type Step = { action: string; user: { uuid: string; name: string } | null };

/** The user who opened the order: the `created` step of the timeline. */
export function creatorOf(timeline: readonly Step[] | undefined): { uuid: string; name: string } | null {
  return timeline?.find((s) => s.action === 'created')?.user ?? null;
}

/** Newest first, for the detail list (the API sends oldest first). */
export function newestFirst<T extends { at: string }>(timeline: readonly T[] | undefined): T[] {
  return [...(timeline ?? [])].sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
}

/** Seller warehouse filter of `GET /v1/orders` (TEC-1054); nothing when no warehouse is known. */
export function sellerWarehouseFilter(warehouse: string | undefined): { seller_warehouse?: string } {
  return warehouse ? { seller_warehouse: warehouse } : {};
}

type StockRow = { product: { uuid: string }; quantity: number; meters: string };

/** On hand amount per product, from the stock products rows of one organization. */
export function stockByProduct(rows: readonly StockRow[]): Map<string, { quantity: number; meters: number }> {
  const map = new Map<string, { quantity: number; meters: number }>();
  for (const r of rows) {
    const meters = Number(r.meters);
    map.set(r.product.uuid, { quantity: r.quantity, meters: Number.isFinite(meters) ? meters : 0 });
  }
  return map;
}

/**
 * Stock state of a catalog row. `complete` says whether the rows cover every
 * product in stock; when they do not, a product missing from them is unknown,
 * not out of stock.
 */
export function stockState(
  map: Map<string, { quantity: number; meters: number }> | undefined,
  product: string,
  complete: boolean,
): { kind: 'in'; quantity: number; meters: number } | { kind: 'out' } | { kind: 'unknown' } {
  if (!map) return { kind: 'unknown' };
  const s = map.get(product);
  if (s && (s.quantity > 0 || s.meters > 0)) return { kind: 'in', ...s };
  return complete || s ? { kind: 'out' } : { kind: 'unknown' };
}
