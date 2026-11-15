/**
 * Chart geometry: nice axis ticks, linear and band scales, label thinning and
 * the touch → bucket lookup of the crosshair. Pure, so node --test runs it.
 */

/** A "nice" step (1, 2, 2.5 or 5 × 10^n) close to `raw`. */
export function niceStep(raw: number): number {
  if (!Number.isFinite(raw) || raw <= 0) return 1;
  const exp = Math.floor(Math.log10(raw));
  const base = 10 ** exp;
  const f = raw / base;
  const nice = f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10;
  return nice * base;
}

/**
 * Y axis ticks from 0 (or the negative minimum) up past the maximum, about
 * `count` intervals with a nice step. A flat or empty domain still gets an axis
 * (0..1) so the chart never divides by zero.
 */
export function niceTicks(min: number, max: number, count = 4): number[] {
  let lo = Math.min(0, Number.isFinite(min) ? min : 0);
  let hi = Math.max(0, Number.isFinite(max) ? max : 0);
  if (hi === lo) hi = lo + 1;
  const step = niceStep((hi - lo) / Math.max(1, count));
  lo = Math.floor(lo / step) * step;
  hi = Math.ceil(hi / step) * step;
  const ticks: number[] = [];
  // Rounded to the step's decimals so 0.1 + 0.2 never shows as 0.30000000000000004.
  const decimals = Math.max(0, -Math.floor(Math.log10(step)) + 1);
  for (let v = lo; v <= hi + step / 2; v += step) ticks.push(Number(v.toFixed(decimals)));
  return ticks;
}

/** Maps a value of [d0, d1] onto [r0, r1]; a zero-width domain maps to r0. */
export function linear(d0: number, d1: number, r0: number, r1: number) {
  const span = d1 - d0;
  return (v: number) => (span === 0 ? r0 : r0 + ((v - d0) / span) * (r1 - r0));
}

/** x of the i-th of n points spread edge to edge over `width` (a single point sits in the middle). */
export function pointX(i: number, n: number, width: number): number {
  if (n <= 1) return width / 2;
  return (i / (n - 1)) * width;
}

/**
 * Band layout of grouped bars: n buckets over `width`, each bucket holds
 * `groups` bars side by side. Returns the bucket width, the bar width and
 * the x of bar g in bucket i.
 */
export function bandLayout(n: number, groups: number, width: number, gapRatio = 0.25) {
  const bucket = n > 0 ? width / n : width;
  const inner = bucket * (1 - gapRatio);
  const g = Math.max(1, groups);
  const bar = Math.max(1, inner / g);
  return {
    bucket,
    bar,
    x: (i: number, gi: number) => i * bucket + (bucket - inner) / 2 + gi * bar,
    center: (i: number) => i * bucket + bucket / 2,
  };
}

/** Bucket under a touch at `x`: the nearest point (line) or the band it falls in (bars). */
export function nearestIndex(x: number, n: number, width: number, mode: 'point' | 'band'): number {
  if (n <= 0 || width <= 0) return -1;
  const clamped = Math.min(width, Math.max(0, x));
  if (mode === 'band') return Math.min(n - 1, Math.floor((clamped / width) * n));
  if (n === 1) return 0;
  return Math.round((clamped / width) * (n - 1));
}

/**
 * Indexes of the x labels to draw so at most `max` fit: always the first and
 * the last, evenly spaced in between.
 */
export function labelIndices(n: number, max: number): number[] {
  if (n <= 0) return [];
  if (n <= max) return Array.from({ length: n }, (_, i) => i);
  if (max <= 1) return [0];
  const out = new Set<number>();
  for (let k = 0; k < max; k++) out.add(Math.round((k * (n - 1)) / (max - 1)));
  return [...out].sort((a, b) => a - b);
}

/** Largest absolute value over several series (ignores null and NaN). */
export function extent(values: readonly (number | null | undefined)[]): { min: number; max: number } {
  let min = Infinity;
  let max = -Infinity;
  for (const v of values) {
    if (v === null || v === undefined || !Number.isFinite(v)) continue;
    if (v < min) min = v;
    if (v > max) max = v;
  }
  return Number.isFinite(min) ? { min, max } : { min: 0, max: 0 };
}

/** SVG path through the points; a null value breaks the line (gap, no fake zero). */
export function linePath(points: readonly ({ x: number; y: number } | null)[]): string {
  let d = '';
  let pen = false;
  for (const p of points) {
    if (!p) {
      pen = false;
      continue;
    }
    d += `${pen ? 'L' : 'M'}${round(p.x)},${round(p.y)}`;
    pen = true;
  }
  return d;
}

function round(v: number) {
  return Math.round(v * 10) / 10;
}

/**
 * Donut slices: start/end angles (radians, clockwise from 12 o'clock) of each
 * value's share. Zero and negative values get no slice.
 */
export function donutAngles(values: readonly number[]): { start: number; end: number }[] {
  const total = values.reduce((a, v) => a + (v > 0 ? v : 0), 0);
  let at = 0;
  return values.map((v) => {
    const share = total > 0 && v > 0 ? (v / total) * Math.PI * 2 : 0;
    const slice = { start: at, end: at + share };
    at += share;
    return slice;
  });
}

/** SVG arc path of a donut slice between radii r0 (inner) and r1 (outer), centered at (c, c). */
export function arcPath(c: number, r0: number, r1: number, start: number, end: number): string {
  const sweep = end - start;
  if (sweep <= 0) return '';
  // A full circle cannot be one arc; stop just short of it.
  const e = sweep >= Math.PI * 2 ? start + Math.PI * 2 - 1e-4 : end;
  const large = e - start > Math.PI ? 1 : 0;
  const pt = (r: number, a: number) => `${round(c + r * Math.sin(a))},${round(c - r * Math.cos(a))}`;
  return [
    `M${pt(r1, start)}`,
    `A${r1},${r1} 0 ${large} 1 ${pt(r1, e)}`,
    `L${pt(r0, e)}`,
    `A${r0},${r0} 0 ${large} 0 ${pt(r0, start)}`,
    'Z',
  ].join(' ');
}
