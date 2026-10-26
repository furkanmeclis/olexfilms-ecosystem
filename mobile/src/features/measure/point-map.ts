/**
 * NexPTG part map on the phone (TEC-1010): the main view of each place with
 * every part body in the mean level color and the numbered points in their
 * level color, plus the thickness of every numbered point per part. A
 * string port of the panel's part-map.ts / thresholds.ts (React Native has
 * no DOMParser); the body type SVGs come from GET
 * /v1/measurement-part-maps/{body_type} and all share one layout: the part
 * body in <g id="PART"> followed by <g id="PART_points">.
 */

export const PLACES = ['left', 'right', 'top', 'back'] as const;

export type MapPoint = { count: number | null; x: number; y: number };
export type MapAsset = {
  part: string;
  place: string;
  kind: string;
  points: MapPoint[];
  svg: string | null;
};
export type PartMapData = { point_radius: number; assets: MapAsset[] };

export type MapValue = {
  part_type: string;
  position: number | null;
  interpretation: number | null;
  value_um: string | null;
  is_inside: boolean;
};

const COLORS: Record<number, string> = {
  [-1]: '#9CA3AF',
  0: '#e9df28',
  1: '#55d37a',
  2: '#deb50a',
  3: '#ec4a08',
  4: '#af0025',
  5: '#af0025',
};

/** Interpretation level: null and unknown codes are -1. */
export function level(v: number | null | undefined): number {
  return v !== null && v !== undefined && v in COLORS ? v : -1;
}

export function levelColor(v: number | null | undefined): string {
  return COLORS[level(v)];
}

/** Rounded mean level of a part (null and -1 excluded), clamped to 0..5. */
export function averageLevel(values: readonly MapValue[], part: string): number | null {
  let sum = 0;
  let n = 0;
  for (const v of values) {
    if (v.part_type !== part || level(v.interpretation) === -1) continue;
    sum += v.interpretation as number;
    n++;
  }
  if (n === 0) return null;
  return level(Math.min(5, Math.max(0, Math.floor(sum / n + 0.5))));
}

/** Level per point position; a later reading of a position wins. */
export function levelsByPosition(values: readonly MapValue[], part: string): Map<number, number> {
  const out = new Map<number, number>();
  for (const v of values) {
    if (v.part_type !== part || v.position === null) continue;
    out.set(v.position, level(v.interpretation));
  }
  return out;
}

const WHITE = /fill="(#FFFFFF|#ffffff|#FFF|#fff|white)"/g;

/** The part body of a part SVG in a color, without metadata and points. */
export function partBody(svg: string, part: string, color: string): string {
  const start = svg.indexOf(`<g id="${part}">`);
  const end = svg.indexOf(`<g id="${part}_points">`);
  if (start < 0 || end < start) return '';
  return svg
    .slice(start, end)
    .replace(/<metadata>[\s\S]*?<\/metadata>/g, '')
    .replace(WHITE, `fill="${color}"`);
}

const num = (v: number) => String(Number.isInteger(v) ? v : Number(v.toPrecision(14)));

/** The main view of a place with the part bodies and numbered points; null when the body type has none. */
export function composeView(map: PartMapData, place: string, values: readonly MapValue[]): string | null {
  const main = map.assets.find((a) => a.place === place && a.kind === 'main');
  if (!main?.svg) return null;
  let overlay = '';
  let points = '';
  for (const a of map.assets) {
    if (a.place !== place || a.kind !== 'element' || !a.part) continue;
    const avg = averageLevel(values, a.part);
    if (a.svg && avg !== null) {
      const body = partBody(a.svg, a.part, levelColor(avg));
      if (body) overlay += `<g opacity="0.72">${body}</g>`;
    }
    const levels = levelsByPosition(values, a.part);
    for (const p of a.points) {
      if (p.count === null || p.count < 1) continue;
      const fill = levelColor(levels.get(p.count) ?? -1);
      points +=
        `<circle cx="${num(p.x)}" cy="${num(p.y)}" r="${num(map.point_radius)}" fill="${fill}" stroke="${fill}" stroke-width="2"/>` +
        `<text x="${num(p.x)}" y="${num(p.y)}" dy="7" text-anchor="middle" font-size="19" font-weight="700" fill="#111827">${p.count}</text>`;
    }
  }
  const svg = main.svg.replace(/<\?xml[^>]*\?>/, '').replace(/<!--[\s\S]*?-->/g, '');
  if (!overlay && !points) return svg;
  return svg.split('</svg>').join(`<g>${overlay}${points}</g></svg>`);
}

export type PointValue = { position: number; um: number | null; level: number };
export type PartPoints = {
  part: string;
  outside: PointValue[];
  inside: PointValue[];
};

function sidePoints(values: readonly MapValue[], part: string, inside: boolean): PointValue[] {
  const by = new Map<number, PointValue>();
  for (const v of values) {
    if (v.part_type !== part || v.position === null || v.is_inside !== inside) continue;
    const um = v.value_um === null ? null : Number(v.value_um);
    by.set(v.position, {
      position: v.position,
      um: um !== null && Number.isFinite(um) ? um : null,
      level: level(v.interpretation),
    });
  }
  return [...by.values()].sort((a, b) => a.position - b.position);
}

/** Parts with readings in manifest order, then parts the body type does not draw. */
export function partPoints(map: PartMapData | null | undefined, values: readonly MapValue[]): PartPoints[] {
  const out: PartPoints[] = [];
  const seen = new Set<string>();
  const add = (part: string) => {
    if (seen.has(part)) return;
    seen.add(part);
    const outside = sidePoints(values, part, false);
    const inside = sidePoints(values, part, true);
    if (outside.length || inside.length) out.push({ part, outside, inside });
  };
  for (const a of map?.assets ?? []) if (a.kind === 'element') add(a.part);
  for (const v of values) add(v.part_type);
  return out;
}
