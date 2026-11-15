/**
 * TEC-1211 data-viz palette. Values live as CSS variables in
 * styles/theme.css (light and dark are separate steps); this module only
 * hands out `var(--viz-*)` references so every chart follows the theme.
 *
 * Rules:
 * - Categorical order is fixed (slot 1..8) and never cycles. A 9th or later
 *   series is folded into "Other" ({@link foldSeries}).
 * - Colour follows identity, not position: {@link assignColorSlots} keeps a
 *   series on the slot it got first, so filtering does not repaint others.
 * - Status colours are only for status (with icon + label), never series.
 */

export const CATEGORICAL_SLOTS = 8;
export const SEQUENTIAL_STEPS = 13;
export const DIVERGING_STEPS = 11;
/** Forms where every pair touches (scatter, dot plot) take at most 3. */
export const MAX_DENSE_SERIES = 3;
export const OTHER_KEY = "__other";

/** Literal hex values (tests, XLSX/print export). Mirrors theme.css. */
export const CATEGORICAL_HEX = {
  light: [
    "#2a78d6",
    "#eb6834",
    "#1baf7a",
    "#eda100",
    "#e87ba4",
    "#008300",
    "#6250d6",
    "#e34948",
  ],
  dark: [
    "#3987e5",
    "#d95926",
    "#199e70",
    "#c98500",
    "#d55181",
    "#008300",
    "#9085e9",
    "#e66767",
  ],
} as const;

export const SEQUENTIAL_HEX = [
  "#cde2fb",
  "#b7d3f6",
  "#9ec5f4",
  "#86b6ef",
  "#6da7ec",
  "#5598e7",
  "#3987e5",
  "#2a78d6",
  "#256abf",
  "#1c5cab",
  "#184f95",
  "#104281",
  "#0d366b",
] as const;

export const STATUS_HEX = {
  good: "#0ca30c",
  warning: "#fab219",
  serious: "#ec835a",
  critical: "#d03b3b",
} as const;

export type StatusLevel = keyof typeof STATUS_HEX;

export const vizTokens = {
  surface: "var(--viz-surface)",
  grid: "var(--viz-grid)",
  axis: "var(--viz-axis)",
  muted: "var(--viz-muted)",
  other: "var(--viz-other)",
  deltaGood: "var(--viz-delta-good)",
  deltaBad: "var(--viz-delta-bad)",
} as const;

/** CSS colour of categorical slot (0-based). Out of range → "Other". */
export function categoricalColor(slot: number): string {
  if (!Number.isInteger(slot) || slot < 0 || slot >= CATEGORICAL_SLOTS) {
    return vizTokens.other;
  }
  return `var(--viz-cat-${slot + 1})`;
}

export function statusColor(level: StatusLevel): string {
  return `var(--viz-${level})`;
}

/** Sequential (magnitude) step for t in [0, 1]. */
export function sequentialColor(t: number): string {
  return `var(--viz-seq-${sequentialStep(t) + 1})`;
}

/** 0-based sequential step for t in [0, 1] (clamped). */
export function sequentialStep(t: number): number {
  if (!Number.isFinite(t) || t <= 0) return 0;
  if (t >= 1) return SEQUENTIAL_STEPS - 1;
  return Math.min(SEQUENTIAL_STEPS - 1, Math.floor(t * SEQUENTIAL_STEPS));
}

/** Diverging step for t in [-1, 1]: -1 blue, 0 neutral, 1 red. */
export function divergingColor(t: number): string {
  const v = Number.isFinite(t) ? Math.max(-1, Math.min(1, t)) : 0;
  const half = (DIVERGING_STEPS - 1) / 2;
  const step = Math.round(v * half) + half;
  return `var(--viz-div-${step + 1})`;
}

/**
 * Stable slot assignment: keys already in `previous` keep their slot; new
 * keys take the lowest free slot. Keys beyond the 8 slots get -1 (Other).
 * Returns a new map (previous is not mutated) that still remembers keys
 * missing from `keys`, so a series that is filtered out and comes back gets
 * its old colour.
 */
export function assignColorSlots(
  keys: readonly string[],
  previous: Readonly<Record<string, number>> = {},
): Record<string, number> {
  const next: Record<string, number> = { ...previous };
  const used = new Set(
    Object.values(next).filter((slot) => slot >= 0 && slot < CATEGORICAL_SLOTS),
  );
  for (const key of keys) {
    if (key in next) continue;
    let slot = -1;
    for (let i = 0; i < CATEGORICAL_SLOTS; i++) {
      if (!used.has(i)) {
        slot = i;
        break;
      }
    }
    next[key] = slot;
    if (slot >= 0) used.add(slot);
  }
  return next;
}

export type FoldableSeries = { key: string; label: string };

/**
 * Keeps the first `max` series (default 8) and folds the 9th and later into one
 * "Other" series whose value is the row-wise sum. Rows are copied.
 */
export function foldSeries<S extends FoldableSeries>(
  rows: readonly Record<string, unknown>[],
  series: readonly S[],
  otherLabel: string,
  max = CATEGORICAL_SLOTS,
): { rows: Record<string, unknown>[]; series: (S | FoldableSeries)[] } {
  if (series.length <= max) {
    return { rows: rows.map((r) => ({ ...r })), series: [...series] };
  }
  const kept = series.slice(0, max);
  const folded = series.slice(max);
  const out = rows.map((row) => {
    let sum: number | null = null;
    for (const s of folded) {
      const v = row[s.key];
      if (typeof v === "number" && Number.isFinite(v)) sum = (sum ?? 0) + v;
    }
    const copy: Record<string, unknown> = { ...row, [OTHER_KEY]: sum };
    for (const s of folded) delete copy[s.key];
    return copy;
  });
  return {
    rows: out,
    series: [...kept, { key: OTHER_KEY, label: otherLabel }],
  };
}

/**
 * Same as {@link foldSeries} for category data (donut, ranking): keeps the
 * largest `max` items and sums the rest into "Other".
 */
export function foldCategories<T extends { label: string; value: number }>(
  items: readonly T[],
  otherLabel: string,
  max = CATEGORICAL_SLOTS,
): Array<T | { key: string; label: string; value: number }> {
  if (items.length <= max) return [...items];
  const sorted = [...items].sort((a, b) => b.value - a.value);
  const kept = sorted.slice(0, max);
  const rest = sorted.slice(max).reduce((acc, i) => acc + i.value, 0);
  return [...kept, { key: OTHER_KEY, label: otherLabel, value: rest }];
}
