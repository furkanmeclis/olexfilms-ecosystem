/**
 * Price visibility on mobile (TEC-1051, revize 5.4): warehouse people see
 * nothing about prices. Pure, no `@/` imports, so node --test runs it.
 *
 * - In the warehouse space (`/depo`) no price, amount or total is shown, for
 *   anyone: it is the place where orders are prepared, not priced.
 * - A warehouse role (the warehouse is the user's first space: center
 *   warehouse, distributor warehouse staff) sees no prices anywhere unless it
 *   holds a pricing read permission (`pricing.purchase.read` /
 *   `pricing.sale.read`).
 * - Everyone else keeps today's behaviour; a price the API leaves null is
 *   simply not shown (see `hasAmount`).
 */

export const PRICE_READ_PERMISSIONS = ['pricing.purchase.read', 'pricing.sale.read'] as const;

export function pricesVisible(input: {
  permissions: readonly string[];
  superAdmin: boolean;
  /** First space of the user (`kindsOf(me)[0]`). */
  primarySpace: string | undefined;
  /** Current route path. */
  pathname: string;
}): boolean {
  if (input.pathname === '/depo' || input.pathname.startsWith('/depo/')) return false;
  if (input.superAdmin) return true;
  const pricing = PRICE_READ_PERMISSIONS.some((p) => input.permissions.includes(p));
  if (input.primarySpace === 'depo' && !pricing) return false;
  return true;
}

/** An amount worth showing: the API answers null (or an empty string) when the viewer may not see it. */
export function hasAmount(v: string | number | null | undefined): v is string | number {
  if (v === null || v === undefined || v === '') return false;
  return !Number.isNaN(typeof v === 'string' ? Number(v) : v);
}
