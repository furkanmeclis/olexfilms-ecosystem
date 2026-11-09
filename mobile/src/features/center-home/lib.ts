/**
 * Center home boards (TEC-1068 sales, TEC-1073 services). The report API
 * ranks products, brands and models, but has no "dealers by orders" or
 * "dealers by services" ranking and no "dealers without orders" list, so the
 * app derives those from the order / service lists of the period. Pure, no
 * `@/` imports, so node --test runs it.
 */

/** The report periods (reports/api.ts `PERIODS`). */
export type HomePeriod = '7d' | '30d' | '90d' | '12m';

/** Pages of 100 rows read at most for one ranking (the list APIs cap `limit` at 100). */
export const RANK_PAGE = 100;
export const RANK_MAX_PAGES = 5;

const DAYS: Record<Exclude<HomePeriod, '12m'>, number> = { '7d': 7, '30d': 30, '90d': 90 };

/**
 * First day (UTC, YYYY-MM-DD) of a period ending today, for `created_from`:
 * the last 7 / 30 / 90 days including today, or the last 12 months.
 */
export function periodStart(period: HomePeriod, now: Date = new Date()): string {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  if (period === '12m') {
    d.setUTCFullYear(d.getUTCFullYear() - 1);
    d.setUTCDate(d.getUTCDate() + 1);
  } else {
    d.setUTCDate(d.getUTCDate() - (DAYS[period] - 1));
  }
  return d.toISOString().slice(0, 10);
}

type OrgRef = { uuid: string; name: string };

export type RankRow = {
  uuid: string;
  name: string;
  count: number;
  /** Sum of the order totals per currency (orders only; empty when no amount is visible). */
  totals: Record<string, number>;
};

function bump(by: Map<string, RankRow>, org: OrgRef) {
  let row = by.get(org.uuid);
  if (!row) {
    row = { uuid: org.uuid, name: org.name, count: 0, totals: {} };
    by.set(org.uuid, row);
  }
  row.count += 1;
  return row;
}

/** Most first, then name, then uuid (stable). */
function ranked(by: Map<string, RankRow>, limit: number): RankRow[] {
  return [...by.values()]
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name) || (a.uuid < b.uuid ? -1 : a.uuid > b.uuid ? 1 : 0))
    .slice(0, limit);
}

type OrderLike = {
  status: string;
  buyer: OrgRef;
  total?: string | number | null;
  currency?: string | null;
};

/** Orders that count as sales: everything but drafts and cancellations. */
export function isSale(o: { status: string }): boolean {
  return o.status !== 'draft' && o.status !== 'cancelled' && o.status !== 'cancelling';
}

/** Dealers that ordered the most (by number of orders; totals per currency when visible). */
export function topBuyers(orders: readonly OrderLike[], limit = 5): RankRow[] {
  const by = new Map<string, RankRow>();
  for (const o of orders) {
    if (!isSale(o)) continue;
    const row = bump(by, o.buyer);
    const v = o.total === null || o.total === undefined || o.total === '' ? NaN : Number(o.total);
    if (o.currency && Number.isFinite(v)) row.totals[o.currency] = Math.round(((row.totals[o.currency] ?? 0) + v) * 100) / 100;
  }
  return ranked(by, limit);
}

/** Active dealers with no sale in the orders read (name order). */
export function dealersWithoutOrders<D extends { uuid: string; name: string; status?: string | null }>(
  dealers: readonly D[],
  orders: readonly OrderLike[],
): D[] {
  const bought = new Set(orders.filter(isSale).map((o) => o.buyer.uuid));
  return dealers
    .filter((d) => (d.status ?? 'active') === 'active' && !bought.has(d.uuid))
    .sort((a, b) => a.name.localeCompare(b.name));
}

type ServiceLike = { status: string; organization?: (OrgRef & { type?: string | null }) | null };

/** Dealers that opened the most services in the period (cancelled ones left out). */
export function topOpeners(services: readonly ServiceLike[], limit = 5): RankRow[] {
  const by = new Map<string, RankRow>();
  for (const s of services) {
    if (s.status === 'cancelled' || !s.organization) continue;
    if (s.organization.type && s.organization.type !== 'dealer') continue;
    bump(by, s.organization);
  }
  return ranked(by, limit);
}
