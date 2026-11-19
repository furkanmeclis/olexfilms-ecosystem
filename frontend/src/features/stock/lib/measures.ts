import type { components } from "@/generated/api";

/** TEC-1243 (K14): the on-hand split of one product in a scope. */
export type StockMeasures = components["schemas"]["StockMeasures"];

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;
type FormatNumber = (
  value: number | null | undefined,
  options?: Intl.NumberFormatOptions,
) => string;

const METERS: Intl.NumberFormatOptions = {
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
};

/** Whether the scope holds metered units (open rolls, cut pieces, remnants). */
export function hasMetered(m: StockMeasures | null | undefined): boolean {
  return !!m && m.open_rolls + m.cut_pieces + m.remnants > 0;
}

export type MeasurePart = {
  kind: "boxes" | "open" | "cut" | "remnant";
  text: string;
};

/**
 * The stock line of TEC-1243: "Kapalı kutu 12 · Açık 2 rulo / 18,4 m ·
 * Kesik 3 parça / 9 m · Artık 1,2 m". Closed boxes always show; the metered
 * groups only when present, so a box-only product reads "Kapalı kutu 4".
 */
export function measureParts(
  m: StockMeasures,
  t: Translate,
  number: FormatNumber,
): MeasurePart[] {
  const meters = (v: string) => number(Number(v), METERS);
  const parts: MeasurePart[] = [
    {
      kind: "boxes",
      text: t("stock.measure.boxes", { boxes: number(m.boxes) }),
    },
  ];
  if (m.open_rolls > 0) {
    parts.push({
      kind: "open",
      text: t("stock.measure.open", {
        rolls: number(m.open_rolls),
        meters: meters(m.open_meters),
      }),
    });
  }
  if (m.cut_pieces > 0) {
    parts.push({
      kind: "cut",
      text: t("stock.measure.cut", {
        pieces: number(m.cut_pieces),
        meters: meters(m.cut_meters),
      }),
    });
  }
  if (m.remnants > 0) {
    parts.push({
      kind: "remnant",
      text: t("stock.measure.remnant", { meters: meters(m.remnant_meters) }),
    });
  }
  return parts;
}

/** Meters formatted for a cell ("18,4 m"). */
export function metersText(
  value: string,
  t: Translate,
  number: FormatNumber,
): string {
  return t("stock.widget.meters_value", {
    meters: number(Number(value), METERS),
  });
}
