/**
 * Guided measuring (user decision 11.11; NexPTG has no guide mode, the old
 * Olexfilms app had a point wizard that zoomed in): the body type's numbered
 * points in the backend's walking order (part map `guide`); the screen shows the place's view unzoomed with
 * the active part highlighted and the next point ringed. Readings are stored
 * per point (part + NexPTG position) so the detail map shows them.
 */
import type { PartMapData } from './point-map';

export type GuideTarget = { part: string; place: string; position: number; x: number; y: number };

/** A reading taken at a point. `level` is the SAPL level (interpretation). */
export type PointReading = {
  um: number;
  level: number;
  material: 'fe' | 'al';
  at: string;
};

export const targetKey = (t: Pick<GuideTarget, 'part' | 'position'>) => `${t.part}#${t.position}`;

/** The backend's guided walk of the part map (GET /v1/measurement-part-maps, `guide`). */
export function buildTargets(map: PartMapData): GuideTarget[] {
  return map.guide ?? [];
}

/** The first target without a reading after `from` (then from the start); -1 when every point is done. */
export function nextOpen(targets: GuideTarget[], readings: Record<string, PointReading>, from: number): number {
  const n = targets.length;
  for (let k = 1; k <= n; k += 1) {
    const i = (from + k) % n;
    if (!readings[targetKey(targets[i]!)]) return i;
  }
  return -1;
}

/** First index of the part after `from` (skip part); -1 when it is the last part. */
export function nextPartIndex(targets: GuideTarget[], from: number): number {
  const part = targets[from]?.part;
  for (let i = from + 1; i < targets.length; i += 1) if (targets[i]!.part !== part) return i;
  return -1;
}

/** Parts in guide order with their done / total points. */
export function partProgress(targets: GuideTarget[], readings: Record<string, PointReading>) {
  const out: { part: string; first: number; done: number; total: number }[] = [];
  targets.forEach((t, i) => {
    let row = out.find((r) => r.part === t.part);
    if (!row) {
      row = { part: t.part, first: i, done: 0, total: 0 };
      out.push(row);
    }
    row.total += 1;
    if (readings[targetKey(t)]) row.done += 1;
  });
  return out;
}

const n = (v: number) => String(Number.isInteger(v) ? v : Number(v.toFixed(2)));
const WHITE = /fill="(#FFFFFF|#ffffff|#FFF|#fff|white)"/g;

function body(svg: string, part: string, color: string): string {
  const start = svg.indexOf(`<g id="${part}">`);
  const end = svg.indexOf(`<g id="${part}_points">`);
  if (start < 0 || end < start) return '';
  return svg
    .slice(start, end)
    .replace(/<metadata>[\s\S]*?<\/metadata>/g, '')
    .replace(WHITE, `fill="${color}"`);
}

/**
 * The place view of the current target: the main SVG, the active part filled
 * with `highlight`, measured points of the place in their level color, the
 * active part's open points numbered, and the current point ringed.
 */
export function guideView(
  map: PartMapData,
  current: GuideTarget,
  readings: Record<string, PointReading>,
  colors: { highlight: string; ring: string; levelColor: (level: number) => string },
): string | null {
  const main = map.assets.find((a) => a.place === current.place && a.kind === 'main');
  if (!main?.svg) return null;
  const r = map.point_radius || 22;
  let overlay = '';
  let points = '';
  for (const a of map.assets) {
    if (a.place !== current.place || a.kind !== 'element' || !a.part) continue;
    const active = a.part === current.part;
    if (active && a.svg) {
      const b = body(a.svg, a.part, colors.highlight);
      if (b) overlay += `<g opacity="0.55">${b}</g>`;
    }
    for (const p of a.points) {
      if (p.count === null || p.count < 1) continue;
      const reading = readings[targetKey({ part: a.part, position: p.count })];
      const isCurrent = active && p.count === current.position;
      if (reading) {
        const fill = colors.levelColor(reading.level);
        points += `<circle cx="${n(p.x)}" cy="${n(p.y)}" r="${n(active ? r : r * 0.7)}" fill="${fill}" stroke="#111827" stroke-width="2"/>`;
      } else if (active) {
        points += `<circle cx="${n(p.x)}" cy="${n(p.y)}" r="${n(r)}" fill="#FFFFFF" stroke="#111827" stroke-width="3"/>`;
      } else {
        continue;
      }
      if (isCurrent) {
        points += `<circle cx="${n(p.x)}" cy="${n(p.y)}" r="${n(r * 1.9)}" fill="none" stroke="${colors.ring}" stroke-width="8"/>`;
      }
      if (active) {
        points += `<text x="${n(p.x)}" y="${n(p.y)}" dy="7" text-anchor="middle" font-size="20" font-weight="700" fill="#111827">${p.count}</text>`;
      }
    }
  }
  const svg = main.svg.replace(/<\?xml[^>]*\?>/, '').replace(/<!--[\s\S]*?-->/g, '');
  return svg.split('</svg>').join(`<g>${overlay}${points}</g></svg>`);
}
