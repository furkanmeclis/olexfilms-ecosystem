/**
 * NexPTG interpretation levels and their colors: the single source of the
 * part map coloring in the panel (TEC-299). Port of the backend
 * `backend/internal/modules/measurements/svg/fill.go` (itself the legacy
 * NexptgInterpretationEnum); keep both in step. The device interprets each
 * reading; the map shows the point in its level color and the part body in
 * the rounded mean level of its readings.
 */
export const Interpretation = {
  Unknown: -1,
  TooThin: 0,
  Original: 1,
  SecondLayer: 2,
  ThinPutty: 3,
  ThickPutty: 4,
  ThickPuttyHigh: 5,
} as const;

export type InterpretationLevel =
  (typeof Interpretation)[keyof typeof Interpretation];

const COLORS: Record<InterpretationLevel, string> = {
  [-1]: "#9CA3AF",
  0: "#e9df28",
  1: "#55d37a",
  2: "#deb50a",
  3: "#ec4a08",
  4: "#af0025",
  5: "#af0025",
};

/** Legend rows (the PDF legend: thick putty covers levels 4 and 5). */
export const LEGEND: { level: InterpretationLevel; key: string }[] = [
  { level: Interpretation.TooThin, key: "too_thin" },
  { level: Interpretation.Original, key: "original" },
  { level: Interpretation.SecondLayer, key: "second_layer" },
  { level: Interpretation.ThinPutty, key: "thin_putty" },
  { level: Interpretation.ThickPutty, key: "thick_putty" },
  { level: Interpretation.Unknown, key: "unknown" },
];

/** fromValue: null and unknown codes are Unknown. */
export function interpretationLevel(
  value: number | null | undefined,
): InterpretationLevel {
  if (value === null || value === undefined) return Interpretation.Unknown;
  return value in COLORS ? (value as InterpretationLevel) : -1;
}

/** Fill color of an interpretation code (unknown codes are gray). */
export function interpretationColor(value: number | null | undefined): string {
  return COLORS[interpretationLevel(value)];
}

/** One reading as the map needs it (measurement_values row). */
export type MapReading = {
  part_type: string;
  position: number | null;
  interpretation: number | null;
};

/**
 * Rounded mean level of a part's readings (null and -1 excluded), clamped
 * to 0..5; null without readings. Rounds half away from zero like Go's
 * math.Round (the mean is never negative here).
 */
export function averagePartInterpretation(
  readings: readonly MapReading[],
  part: string,
): InterpretationLevel | null {
  let sum = 0;
  let n = 0;
  for (const r of readings) {
    if (r.part_type !== part) continue;
    if (interpretationLevel(r.interpretation) === Interpretation.Unknown) {
      continue;
    }
    sum += r.interpretation as number;
    n++;
  }
  if (n === 0) return null;
  const rounded = Math.min(
    Interpretation.ThickPuttyHigh,
    Math.max(Interpretation.TooThin, Math.floor(sum / n + 0.5)),
  );
  return interpretationLevel(rounded);
}

/**
 * Level per point position of a part, in first-seen position order; a
 * later reading of the same position overwrites the level.
 */
export function levelsByPosition(
  readings: readonly MapReading[],
  part: string,
): Map<number, InterpretationLevel> {
  const out = new Map<number, InterpretationLevel>();
  for (const r of readings) {
    if (r.part_type !== part || r.position === null) continue;
    out.set(r.position, interpretationLevel(r.interpretation));
  }
  return out;
}
