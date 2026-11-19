import { describe, expect, it } from "vitest";

import tr from "@/locales/tr/stock.json";

import { hasMetered, measureParts, type StockMeasures } from "./measures";

const dict = tr as Record<string, string>;
const t = (key: string, vars?: Record<string, string | number>) =>
  (dict[key.replace(/^stock\./, "")] ?? key).replace(/\{\{(\w+)\}\}/g, (_, k) =>
    String(vars?.[k] ?? ""),
  );
const number = (v: number | null | undefined, o?: Intl.NumberFormatOptions) =>
  new Intl.NumberFormat("tr-TR", o).format(v ?? 0);

const m: StockMeasures = {
  boxes: 12,
  available_boxes: 10,
  open_rolls: 2,
  open_meters: "18.40",
  cut_pieces: 3,
  cut_meters: "9.00",
  remnants: 1,
  remnant_meters: "1.20",
  available_meters: "27.40",
};

describe("stock measure line (TEC-1243, K14)", () => {
  it("reads boxes, open rolls, cut pieces and remnants at a glance", () => {
    expect(
      measureParts(m, t, number)
        .map((p) => p.text)
        .join(" · "),
    ).toBe(
      "Kapalı kutu 12 · Açık 2 rulo / 18,4 m · Kesik 3 parça / 9 m · Artık 1,2 m",
    );
  });

  it("shows only the closed boxes of a box-only product", () => {
    const boxOnly = {
      ...m,
      open_rolls: 0,
      cut_pieces: 0,
      remnants: 0,
    };
    expect(hasMetered(boxOnly)).toBe(false);
    expect(measureParts(boxOnly, t, number).map((p) => p.kind)).toEqual([
      "boxes",
    ]);
    expect(hasMetered(m)).toBe(true);
    expect(hasMetered(undefined)).toBe(false);
  });
});
