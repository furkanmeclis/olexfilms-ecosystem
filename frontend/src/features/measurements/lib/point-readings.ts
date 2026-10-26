import type { PartMapData } from "@/features/measurements/lib/part-map";
import {
  interpretationLevel,
  type InterpretationLevel,
  type MapReading,
} from "@/features/measurements/lib/thresholds";

/**
 * Readings per numbered point (TEC-1010): next to the part map each part
 * lists its points with the measured thickness, outside and inside
 * separately, so the µm value of every numbered circle is readable.
 */

export type ThicknessUnit = "um" | "mil";

/** 1 mil = 25.4 µm. */
export const MICRONS_PER_MIL = 25.4;

export function convertThickness(um: number, unit: ThicknessUnit): number {
  return unit === "mil" ? um / MICRONS_PER_MIL : um;
}

/** Fraction digits of a unit: µm is shown whole, mil with two places. */
export function unitDigits(unit: ThicknessUnit): number {
  return unit === "mil" ? 2 : 0;
}

export type PointValue = {
  position: number;
  valueUm: number | null;
  level: InterpretationLevel;
};

export type PartPoints = {
  part: string;
  place: string;
  outside: PointValue[];
  inside: PointValue[];
};

function points(
  readings: readonly MapReading[],
  part: string,
  inside: boolean,
): PointValue[] {
  // A later reading of the same position wins, like the map coloring.
  const byPosition = new Map<number, PointValue>();
  for (const r of readings) {
    if (r.part_type !== part || r.position === null) continue;
    if (Boolean(r.is_inside) !== inside) continue;
    const value =
      r.value_um === null || r.value_um === undefined
        ? null
        : Number(r.value_um);
    byPosition.set(r.position, {
      position: r.position,
      valueUm: value !== null && Number.isFinite(value) ? value : null,
      level: interpretationLevel(r.interpretation),
    });
  }
  return [...byPosition.values()].sort((a, b) => a.position - b.position);
}

/**
 * The element parts of the body type that have readings, in manifest
 * order, then readings of parts the body type does not draw (so no value
 * is ever hidden).
 */
export function partPoints(
  map: PartMapData | null | undefined,
  readings: readonly MapReading[],
): PartPoints[] {
  const out: PartPoints[] = [];
  const seen = new Set<string>();
  const add = (part: string, place: string) => {
    if (seen.has(part)) return;
    seen.add(part);
    const outside = points(readings, part, false);
    const inside = points(readings, part, true);
    if (outside.length > 0 || inside.length > 0) {
      out.push({ part, place, outside, inside });
    }
  };
  for (const a of map?.assets ?? []) {
    if (a.kind === "element" && a.part) add(a.part, a.place);
  }
  for (const r of readings) add(r.part_type, "");
  return out;
}
