import {
  Interpretation,
  averagePartInterpretation,
  interpretationColor,
  levelsByPosition,
  type MapReading,
} from "@/features/measurements/lib/thresholds";

/**
 * NexPTG part map coloring in the browser (TEC-299): a port of the
 * backend `measurements/svg` FillPart / ComposePlaceView, run on the raw
 * body type SVGs of GET /v1/measurement-part-maps/{body_type}. Colors come
 * from `thresholds.ts` only.
 */

export const PLACES = ["left", "right", "top", "back"] as const;
export type Place = (typeof PLACES)[number];

export type PartMapPoint = {
  count: number | null;
  x: number;
  y: number;
};

export type PartMapAsset = {
  part: string;
  place: string;
  kind: string;
  points: PartMapPoint[];
  svg: string | null;
};

export type PartMapData = {
  point_radius: number;
  assets: PartMapAsset[];
};

const WHITE = new Set(["#FFFFFF", "#ffffff", "#FFF", "#fff", "white"]);
const SVG_NS = ` xmlns="http://www.w3.org/2000/svg"`;

function parseSvg(src: string): Document | null {
  const doc = new DOMParser().parseFromString(src, "image/svg+xml");
  if (doc.getElementsByTagName("parsererror").length > 0) return null;
  return doc;
}

/** Elements with the id, the root first, in document order. */
function allById(doc: Document, id: string): Element[] {
  return Array.from(doc.getElementsByTagName("*")).filter(
    (el) => el.getAttribute("id") === id,
  );
}

function isInsidePointsGroup(el: Element, part: string): boolean {
  for (let cur = el.parentElement; cur; cur = cur.parentElement) {
    const id = cur.getAttribute("id") ?? "";
    if (id === `${part}_points` || id.includes("_point_")) return true;
    if (id === part) return false;
  }
  return false;
}

function serialize(el: Element): string {
  return new XMLSerializer().serializeToString(el);
}

/**
 * A part SVG with the point circles in their level color and the white
 * part body in the mean level color. Without readings the source is
 * returned unchanged.
 */
export function fillPart(
  src: string,
  part: string,
  readings: readonly MapReading[],
): string {
  const byPosition = levelsByPosition(readings, part);
  const average = averagePartInterpretation(readings, part);
  if (byPosition.size === 0 && average === null) return src;
  const doc = parseSvg(src);
  if (!doc) return src;
  if (average !== null) {
    const color = interpretationColor(average);
    const seen = new Set<Element>();
    for (const holder of allById(doc, part)) {
      for (const el of Array.from(holder.getElementsByTagName("*"))) {
        if (seen.has(el)) continue;
        seen.add(el);
        const fill = el.getAttribute("fill");
        if (fill === null || !WHITE.has(fill)) continue;
        if (isInsidePointsGroup(el, part)) continue;
        el.setAttribute("fill", color);
      }
    }
  }
  for (const [position, level] of byPosition) {
    const group = allById(doc, `${part}_point_${position}`)[0];
    if (!group) continue;
    const color = interpretationColor(level);
    for (const el of Array.from(group.getElementsByTagName("*"))) {
      if (el.localName !== "circle") continue;
      el.setAttribute("fill", color);
      el.setAttribute("stroke", color);
    }
  }
  return serialize(doc.documentElement);
}

/** The part body of a filled part SVG without its points and metadata. */
function partBodyMarkup(filled: string, part: string): string {
  const doc = parseSvg(filled);
  const holder = doc ? allById(doc, part)[0] : undefined;
  if (!holder) return "";
  let out = "";
  for (const child of Array.from(holder.children)) {
    const id = child.getAttribute("id") ?? "";
    if (id === `${part}_points` || id.includes("_point_")) continue;
    if (child.localName.toLowerCase() === "metadata") continue;
    out += serialize(child).replace(SVG_NS, "") + "\n";
  }
  return out;
}

/** PHP's (string) of a float (precision 14), as the backend prints it. */
export function phpNumber(v: number): string {
  if (Number.isInteger(v)) return String(v);
  return String(Number(v.toPrecision(14)));
}

const esc = (s: string) =>
  s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");

/**
 * The main view of a place with every element part body in its mean color
 * (opacity 0.72) and the numbered measurement points on top; null when the
 * body type has no main view for the place.
 */
export function composePlaceView(
  map: PartMapData,
  place: string,
  readings: readonly MapReading[],
): string | null {
  const main = map.assets.find((a) => a.place === place && a.kind === "main");
  if (!main?.part || !main.svg) return null;
  const radius = phpNumber(map.point_radius);
  let overlays = "";
  let points = "";
  for (const a of map.assets) {
    if (a.place !== place || a.kind !== "element" || !a.part) continue;
    if (a.svg) {
      const body = partBodyMarkup(fillPart(a.svg, a.part, readings), a.part);
      if (body) {
        const p = esc(a.part);
        overlays += `    <g id="nexptg_part_body_${p}" data-part="${p}" opacity="0.72">\n`;
        overlays += body;
        overlays += "\n    </g>\n";
      }
    }
    const levels = levelsByPosition(readings, a.part);
    for (const pt of a.points) {
      if (pt.count === null || pt.count < 1) continue;
      const cx = esc(phpNumber(pt.x));
      const cy = esc(phpNumber(pt.y));
      const fill = esc(
        interpretationColor(levels.get(pt.count) ?? Interpretation.Unknown),
      );
      const label = String(pt.count);
      points += `    <g id="${esc(`${a.part}_point_${label}`)}" data-part="${esc(a.part)}" data-count="${label}">\n`;
      points += `      <circle cx="${cx}" cy="${cy}" r="${esc(radius)}" fill="${fill}" stroke="${fill}" stroke-width="2"/>\n`;
      points += `      <text x="${cx}" y="${cy}" text-anchor="middle" dominant-baseline="central" font-family="Arial, Helvetica, sans-serif" font-size="19" font-weight="700" fill="#111827">${label}</text>\n`;
      points += "    </g>\n";
    }
  }
  if (!overlays && !points) return main.svg;
  let overlay = `<g id="nexptg_measurement_overlay">\n${overlays}`;
  if (points) {
    overlay += `    <g id="nexptg_measurement_points">\n${points}    </g>\n`;
  }
  overlay += "  </g>";
  return main.svg.includes("</svg>")
    ? main.svg.split("</svg>").join(`${overlay}</svg>`)
    : main.svg + overlay;
}

/**
 * Inline markup of a composed view: the root <svg> only (no XML prolog),
 * sized by its viewBox so it scales with the card.
 */
export function toInlineSvg(svg: string): string | null {
  const doc = parseSvg(svg);
  if (!doc) return null;
  const root = doc.documentElement;
  const w = parseFloat(root.getAttribute("width") ?? "");
  const h = parseFloat(root.getAttribute("height") ?? "");
  if (!root.getAttribute("viewBox") && w > 0 && h > 0) {
    root.setAttribute("viewBox", `0 0 ${w} ${h}`);
  }
  root.removeAttribute("width");
  root.removeAttribute("height");
  root.setAttribute("width", "100%");
  root.setAttribute("role", "img");
  return serialize(root);
}
