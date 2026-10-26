import { describe, expect, it } from "vitest";

import type { PartMapData } from "@/features/measurements/lib/part-map";
import {
  convertThickness,
  partPoints,
} from "@/features/measurements/lib/point-readings";
import type { MapReading } from "@/features/measurements/lib/thresholds";

const map: PartMapData = {
  point_radius: 22,
  assets: [
    { part: "LEFT_SIDE", place: "left", kind: "main", points: [], svg: null },
    { part: "HOOD", place: "top", kind: "element", points: [], svg: null },
    { part: "ROOF", place: "top", kind: "element", points: [], svg: null },
  ],
};

const r = (
  part: string,
  position: number,
  value: string | null,
  interpretation: number | null,
  inside = false,
): MapReading => ({
  part_type: part,
  position,
  value_um: value,
  interpretation,
  is_inside: inside,
});

describe("partPoints", () => {
  it("lists the µm of each numbered point per part in manifest order", () => {
    const out = partPoints(map, [
      r("ROOF", 2, "130.00", 1),
      r("HOOD", 3, "250.50", 3),
      r("HOOD", 1, "112.00", 1),
      r("HOOD", 1, "115.00", 2), // a later reading of a point wins
      r("HOOD", 2, "98.00", 1, true),
      r("TRUCK_BED", 1, "90.00", 1), // not drawn by the body type
    ]);
    expect(out.map((p) => p.part)).toEqual(["HOOD", "ROOF", "TRUCK_BED"]);
    expect(out[0].outside).toEqual([
      { position: 1, valueUm: 115, level: 2 },
      { position: 3, valueUm: 250.5, level: 3 },
    ]);
    expect(out[0].inside).toEqual([{ position: 2, valueUm: 98, level: 1 }]);
    expect(out[2].place).toBe("");
  });

  it("keeps a point without a value and skips readings without a position", () => {
    const out = partPoints(null, [
      r("HOOD", 1, null, -1),
      { part_type: "HOOD", position: null, interpretation: 1 },
    ]);
    expect(out).toEqual([
      {
        part: "HOOD",
        place: "",
        outside: [{ position: 1, valueUm: null, level: -1 }],
        inside: [],
      },
    ]);
  });
});

describe("convertThickness", () => {
  it("converts µm to mil", () => {
    expect(convertThickness(254, "mil")).toBeCloseTo(10);
    expect(convertThickness(254, "um")).toBe(254);
  });
});
