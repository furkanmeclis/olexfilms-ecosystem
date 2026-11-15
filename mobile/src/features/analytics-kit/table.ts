/**
 * Report table logic: column kinds, sorting, search and the totals row.
 * Columns follow the report envelope (TEC-1210): `type` (text, number, money,
 * percent, date, datetime, duration, status, org, link; older reports send
 * string / integer), `align`, `aggregate` (sum | avg | none) and `currency`.
 * Pure, so node --test runs it.
 */

export type Column = {
  key: string;
  label: string;
  type: string;
  align?: 'left' | 'right' | null;
  aggregate?: 'sum' | 'avg' | 'none' | null;
  currency?: string | null;
};

export type Row = Record<string, unknown>;

export type SortDir = 'asc' | 'desc';
export type Sort = { key: string; dir: SortDir } | null;

const NUMERIC = new Set(['integer', 'number', 'money', 'percent', 'duration']);
const TEMPORAL = new Set(['date', 'datetime']);

export function isNumeric(col: Pick<Column, 'type'>): boolean {
  return NUMERIC.has(col.type);
}

/** Numbers and money read right-aligned unless the column says otherwise. */
export function alignOf(col: Column): 'left' | 'right' {
  return col.align ?? (isNumeric(col) ? 'right' : 'left');
}

/** Number of a cell (money is a decimal string); null when it is not a number. */
export function num(v: unknown): number | null {
  if (typeof v === 'number') return Number.isFinite(v) ? v : null;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  }
  return null;
}

/** Text of an org / link / status cell: the label when the cell is an object. */
export function textOf(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'object') {
    const o = v as Record<string, unknown>;
    for (const k of ['label', 'name', 'title']) if (typeof o[k] === 'string') return o[k] as string;
    return '';
  }
  return String(v);
}

/** Default direction when a header is first tapped: numbers and dates high to low, text A→Z. */
export function firstDir(col: Column): SortDir {
  return isNumeric(col) || TEMPORAL.has(col.type) ? 'desc' : 'asc';
}

/** Next sort when a header is tapped: first direction → the other → off. */
export function nextSort(current: Sort, col: Column): Sort {
  const first = firstDir(col);
  if (!current || current.key !== col.key) return { key: col.key, dir: first };
  if (current.dir === first) return { key: col.key, dir: first === 'asc' ? 'desc' : 'asc' };
  return null;
}

function compare(a: unknown, b: unknown, col: Column, collator: Intl.Collator): number {
  if (isNumeric(col)) {
    const x = num(a);
    const y = num(b);
    if (x === null && y === null) return 0;
    if (x === null) return 1;
    if (y === null) return -1;
    return x - y;
  }
  if (TEMPORAL.has(col.type)) {
    const x = typeof a === 'string' ? Date.parse(a) : NaN;
    const y = typeof b === 'string' ? Date.parse(b) : NaN;
    if (Number.isNaN(x) && Number.isNaN(y)) return 0;
    if (Number.isNaN(x)) return 1;
    if (Number.isNaN(y)) return -1;
    return x - y;
  }
  const x = textOf(a);
  const y = textOf(b);
  if (!x && !y) return 0;
  if (!x) return 1;
  if (!y) return -1;
  return collator.compare(x, y);
}

/**
 * Rows sorted by one column; empty cells always last, ties keep the report's
 * order (stable). Without a sort the report's own order stays.
 */
export function sortRows<R extends Row>(rows: readonly R[], columns: readonly Column[], sort: Sort, locale = 'tr'): R[] {
  if (!sort) return [...rows];
  const col = columns.find((c) => c.key === sort.key);
  if (!col) return [...rows];
  const collator = new Intl.Collator(locale, { sensitivity: 'base', numeric: true });
  const sign = sort.dir === 'asc' ? 1 : -1;
  return rows
    .map((r, i) => ({ r, i }))
    .sort((a, b) => {
      const va = a.r[col.key];
      const vb = b.r[col.key];
      const empty = (v: unknown) => v === null || v === undefined || v === '';
      // Empty cells stay at the bottom in both directions.
      if (empty(va) !== empty(vb)) return empty(va) ? 1 : -1;
      return sign * compare(va, vb, col, collator) || a.i - b.i;
    })
    .map((x) => x.r);
}

/** Lower case without diacritics, locale aware (Turkish İ/ı), for search. */
export function fold(s: string, locale = 'tr'): string {
  return s
    .toLocaleLowerCase(locale)
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/ı/g, 'i');
}

/** Rows whose text cells contain every word of `q` (any column, case and accent insensitive). */
export function searchRows<R extends Row>(rows: readonly R[], columns: readonly Column[], q: string, locale = 'tr'): R[] {
  const words = fold(q.trim(), locale).split(/\s+/).filter(Boolean);
  if (!words.length) return [...rows];
  return rows.filter((r) => {
    const hay = fold(columns.map((c) => textOf(r[c.key])).join(' '), locale);
    return words.every((w) => hay.includes(w));
  });
}

/** Currency of a money cell: the column's, else the row's `currency`. */
export function currencyOf(col: Column, row: Row): string | null {
  if (col.currency) return col.currency;
  return typeof row.currency === 'string' && row.currency ? row.currency : null;
}

export type Total =
  | { kind: 'number'; value: number }
  /** Money totals per currency (rows may mix currencies). */
  | { kind: 'money'; byCurrency: Record<string, number> };

/** Aggregate a column asks for; without one, money and integer counts sum, the rest none. */
export function aggregateOf(col: Column): 'sum' | 'avg' | 'none' {
  if (col.aggregate) return col.aggregate;
  if (col.type === 'money' || col.type === 'integer') return 'sum';
  return 'none';
}

/**
 * Totals row of the rows shown (after search): sum or average per column,
 * money summed per currency. Columns without an aggregate are left out.
 */
export function totals(rows: readonly Row[], columns: readonly Column[]): Record<string, Total> {
  const out: Record<string, Total> = {};
  for (const col of columns) {
    const agg = aggregateOf(col);
    if (agg === 'none' || !isNumeric(col)) continue;
    if (col.type === 'money') {
      const sums: Record<string, number> = {};
      const counts: Record<string, number> = {};
      for (const r of rows) {
        const v = num(r[col.key]);
        const cur = currencyOf(col, r);
        if (v === null || !cur) continue;
        sums[cur] = (sums[cur] ?? 0) + v;
        counts[cur] = (counts[cur] ?? 0) + 1;
      }
      const byCurrency: Record<string, number> = {};
      for (const cur of Object.keys(sums)) {
        const v = agg === 'avg' ? sums[cur] / counts[cur] : sums[cur];
        byCurrency[cur] = Math.round(v * 100) / 100;
      }
      if (Object.keys(byCurrency).length) out[col.key] = { kind: 'money', byCurrency };
      continue;
    }
    let sum = 0;
    let n = 0;
    for (const r of rows) {
      const v = num(r[col.key]);
      if (v === null) continue;
      sum += v;
      n++;
    }
    if (n === 0) continue;
    const value = agg === 'avg' ? sum / n : sum;
    out[col.key] = { kind: 'number', value: Math.round(value * 1e6) / 1e6 };
  }
  return out;
}
