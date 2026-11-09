/**
 * Price preview of a center order for a dealer (TEC-1065, API TEC-1064
 * `POST /v1/orders/quote`): list price (order currency, USD), the dealer's
 * default discount (first order or standard), the discounted unit price, the
 * line total, the order total and its current TRY value. The line may carry a
 * manual discount or be a gift (charged 0). A preview only: the server prices
 * the order again and freezes the rate on approval. No `@/` imports: node
 * --test runs this file directly.
 */

import { discountInvalid, isRoll, normalizeAmount, type CartLine } from './order-lines.ts';

export type QuoteItem = {
  product_uuid: string;
  list_unit_price: string | null;
  unit_price: string | null;
  price_source: string;
};

export type Quote = {
  currency: string;
  first_order: boolean;
  discount_pct: string | null;
  try_rate: string | null;
  items: QuoteItem[];
};

export type LinePrice = {
  /** Price before the discount; null when the product has none. */
  list: number | null;
  /** Discount applied to this line (manual, else the default); null on a gift or a manual price. */
  discountPct: number | null;
  /** Charged unit price; null when unknown. */
  unit: number | null;
  /** unit × amount; null when the unit price or the amount is unknown. */
  total: number | null;
  gift: boolean;
};

export type QuoteTotals = { total: number; totalTry: number | null; incomplete: boolean };

const num = (v: string | null | undefined): number | null => {
  if (v === null || v === undefined || v.trim() === '') return null;
  const n = Number(normalizeAmount(v));
  return Number.isFinite(n) ? n : null;
};

const round = (v: number, digits: number) => {
  const f = 10 ** digits;
  return Math.round(v * f) / f;
};

/** Amount of the line as a number (pieces or meters); null while invalid. */
function lineAmount(line: CartLine): number | null {
  const v = num(line.amount);
  if (v === null || v <= 0) return null;
  return isRoll(line.unit_type) || Number.isInteger(v) ? v : null;
}

/**
 * Unit and line price of one cart line. Order of precedence, as on the server:
 * gift (0) > manual unit price (charged as is) > manual discount on the list
 * price > the quote's default discounted price.
 */
export function linePrice(line: CartLine, quote: Quote | undefined): LinePrice {
  const item = quote?.items.find((i) => i.product_uuid === line.product_uuid);
  const list = num(item?.list_unit_price);
  const amount = lineAmount(line);
  const withTotal = (p: Omit<LinePrice, 'total'>): LinePrice => ({
    ...p,
    total: p.unit === null || amount === null ? null : round(p.unit * amount, 2),
  });
  if (line.is_gift) return withTotal({ list, discountPct: null, unit: 0, gift: true });
  const manual = num(line.unit_price);
  if (manual !== null) return withTotal({ list, discountPct: null, unit: manual, gift: false });
  const own = discountInvalid(line.discount_pct) ? null : num(line.discount_pct);
  if (own !== null && list !== null) {
    return withTotal({ list, discountPct: own, unit: round(list * (1 - own / 100), 4), gift: false });
  }
  return withTotal({ list, discountPct: num(quote?.discount_pct), unit: num(item?.unit_price), gift: false });
}

/** Order total in the order currency and its current TRY value (null without a rate). */
export function quoteTotals(lines: CartLine[], quote: Quote | undefined): QuoteTotals {
  let total = 0;
  let incomplete = false;
  for (const line of lines) {
    const p = linePrice(line, quote).total;
    if (p === null) incomplete = true;
    else total += p;
  }
  total = round(total, 2);
  const rate = num(quote?.try_rate);
  return { total, totalTry: rate === null ? null : round(total * rate, 2), incomplete };
}
