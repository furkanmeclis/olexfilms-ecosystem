/** Catalog default of pricing.deviation_warning_pct (F5 QUESTIONS S35). */
export const DEFAULT_DEVIATION_THRESHOLD = 15;

/** Parses a decimal string; null when absent or not a number. */
export function toNumber(value: string | number | null | undefined) {
  if (value === null || value === undefined || value === "") return null;
  const n = typeof value === "number" ? value : Number(value);
  return Number.isFinite(n) ? n : null;
}

/**
 * The backend's over_threshold rule: |deviation| >= threshold (either way).
 */
export function isOverThreshold(
  deviationPct: string | number | null | undefined,
  threshold: number,
) {
  const dev = toNumber(deviationPct);
  return dev !== null && Math.abs(dev) >= threshold;
}

/** Calendar day (YYYY-MM-DD) of a date in the browser's time zone. */
export function isoDay(date: Date) {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

export function addDays(day: string, days: number) {
  const [y, m, d] = day.split("-").map(Number);
  return isoDay(new Date(y!, m! - 1, d! + days));
}

const DAY_RE = /^\d{4}-\d{2}-\d{2}$/;

export type EffectiveFromError = "required" | "invalid" | "past";

/**
 * A publication may only take effect today or later (the API refuses an
 * earlier day with 400). ISO days compare as strings.
 */
export function effectiveFromError(
  value: string,
  today: string,
): EffectiveFromError | null {
  if (!value) return "required";
  if (!DAY_RE.test(value)) return "invalid";
  return value < today ? "past" : null;
}

const DECIMAL_RE = /^[0-9]{1,16}(\.[0-9]+)?$/;

/**
 * "1.250,5" / "1250,5" / " 1250.50 " → "1250.50" (NUMERIC(18,2) as the
 * publish API takes it); null when not a non-negative decimal.
 */
export function normalizePublishPrice(raw: string): string | null {
  let v = raw.trim().replace(/\s/g, "");
  if (v.includes(",") && v.includes(".")) v = v.replace(/\./g, "");
  v = v.replace(",", ".");
  if (!DECIMAL_RE.test(v)) return null;
  return (Math.round(Number(v) * 100) / 100).toFixed(2);
}

/** price × (1 + pct/100), rounded to 2 decimals; null for a negative result. */
export function applyPercent(price: string, pct: number): string | null {
  const base = toNumber(price);
  if (base === null || !Number.isFinite(pct)) return null;
  const next = Math.round(base * (100 + pct)) / 100;
  return next < 0 ? null : next.toFixed(2);
}

/** One draft price of the publication basket. */
export type BasketRow = {
  productUuid: string;
  productName: string;
  productSku: string;
  /** ISO2, empty = currency-wide. */
  country: string;
  currency: string;
  price: string;
  /** The price in force when the row was drafted (for the diff). */
  previous: string | null;
};

export function basketKey(r: {
  productUuid: string;
  country: string;
  currency: string;
}) {
  return `${r.productUuid}|${r.country}|${r.currency}`;
}

/** Publish request rows of the basket. */
export function publishRows(rows: readonly BasketRow[]) {
  return rows.map((r) => ({
    product_uuid: r.productUuid,
    country: r.country || null,
    currency: r.currency,
    price: r.price,
  }));
}

/** CSV cell (RFC 4180 quoting). */
function csvCell(value: string) {
  return /[",\n\r]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}

/**
 * Current prices as a CSV in the import's own columns (sku, country,
 * currency, price, effective_from) so an edited file can be uploaded back.
 */
export function recommendedCsv(
  rows: readonly {
    sku: string;
    country: string;
    currency: string;
    price: string;
  }[],
) {
  const lines = ["sku,country,currency,price,effective_from"];
  for (const r of rows) {
    lines.push(
      [r.sku, r.country, r.currency, r.price, ""].map(csvCell).join(","),
    );
  }
  return `${lines.join("\n")}\n`;
}

/**
 * A DATE column ("2026-10-09") as noon UTC, so formatting it in any user
 * time zone keeps the calendar day.
 */
export function dayValue(day: string) {
  return `${day}T12:00:00Z`;
}
