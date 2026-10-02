import { describe, expect, it } from "vitest";

import {
  applyPreset,
  availablePartsOf,
  isPresetActive,
  PART_PRESETS,
  partsForItem,
  partsFromItems,
  togglePart,
} from "./car-parts";
import { CAR_SHAPES } from "./car-shapes";
import {
  normalizeMeters,
  stockErrorKey,
  unitMode,
  validateMeters,
  validateQuantity,
} from "./stock";

const preset = (id: string) => PART_PRESETS.find((p) => p.id === id)!;

describe("car parts (TEC-182)", () => {
  it("draws every part key once", () => {
    const parts = CAR_SHAPES.flatMap((s) => (s.part ? [s.part] : []));
    expect(parts).toHaveLength(20);
    expect(new Set(parts).size).toBe(20);
  });

  it("merges the categories' available parts", () => {
    expect(
      availablePartsOf([
        { available_parts: ["body_kaput", "body_tavan"] },
        { available_parts: ["body_tavan", "window_on_cam"] },
        { available_parts: null },
      ]),
    ).toEqual(["body_kaput", "body_tavan", "window_on_cam"]);
  });

  it("toggles a part", () => {
    expect(togglePart(["body_kaput"], "body_tavan")).toEqual([
      "body_kaput",
      "body_tavan",
    ]);
    expect(togglePart(["body_kaput"], "body_kaput")).toEqual([]);
  });

  it("applies a preset to its own group only and toggles it off", () => {
    const all = [
      "body_kaput",
      "body_on_tampon",
      "body_sol_on_camurluk",
      "body_sag_on_camurluk",
      "body_tavan",
      "window_on_cam",
    ];
    const next = applyPreset(
      ["body_tavan", "window_on_cam"],
      preset("front_three"),
      all,
    );
    expect(next.sort()).toEqual(
      [
        "window_on_cam",
        "body_sag_on_camurluk",
        "body_kaput",
        "body_sol_on_camurluk",
      ].sort(),
    );
    expect(isPresetActive(next, preset("front_three"), all)).toBe(true);
    expect(applyPreset(next, preset("front_three"), all)).toEqual([
      "window_on_cam",
    ]);
  });

  it("skips preset parts that are not available", () => {
    expect(applyPreset([], preset("front_four"), ["body_kaput"])).toEqual([
      "body_kaput",
    ]);
  });

  it("limits an item's parts to its category", () => {
    expect(
      partsForItem(["body_kaput", "window_on_cam"], ["window_on_cam"]),
    ).toEqual(["window_on_cam"]);
    expect(partsForItem(["body_kaput"], null)).toEqual([]);
    expect(
      partsFromItems([
        { applied_parts: ["body_kaput"] },
        { applied_parts: ["body_kaput", "body_tavan"] },
      ]),
    ).toEqual(["body_kaput", "body_tavan"]);
  });
});

describe("stock rules (TEC-182)", () => {
  it("validates meters against the rest of the roll", () => {
    expect(validateMeters("", "50.00")).toBe("required");
    expect(validateMeters("abc", "50.00")).toBe("format");
    expect(validateMeters("1.234", "50.00")).toBe("format");
    expect(validateMeters("0", "50.00")).toBe("positive");
    expect(validateMeters("50.01", "50.00")).toBe("exceeds");
    expect(validateMeters("50", "50.00")).toBeNull();
    expect(validateMeters("12,5", "50.00")).toBeNull();
    expect(validateMeters("999", null)).toBeNull();
  });

  it("normalises meters for the API", () => {
    expect(normalizeMeters("12,5")).toBe("12.50");
    expect(normalizeMeters("3")).toBe("3.00");
    expect(normalizeMeters("0")).toBeNull();
  });

  it("validates pieces against the pieces on hand", () => {
    expect(validateQuantity("", 4)).toBe("required");
    expect(validateQuantity("0", 4)).toBe("format");
    expect(validateQuantity("1.5", 4)).toBe("format");
    expect(validateQuantity("5", 4)).toBe("exceeds");
    expect(validateQuantity("4", 4)).toBeNull();
  });

  it("classifies units", () => {
    const base = { initial_meters: null, product: { unit_type: "piece" } };
    expect(unitMode({ ...base, unit_kind: "fixed" })).toBe("fixed");
    expect(unitMode({ ...base, unit_kind: "serial" })).toBe("piece");
    expect(
      unitMode({
        unit_kind: "serial",
        initial_meters: "50.00",
        product: { unit_type: "roll_meter" },
      }),
    ).toBe("roll");
  });

  it("maps API codes to messages", () => {
    expect(stockErrorKey("SERVICE_UNIT_NOT_AVAILABLE")).toBe(
      "services.stock.errors.SERVICE_UNIT_NOT_AVAILABLE",
    );
    expect(stockErrorKey("HTTP_500")).toBe("services.stock.errors.generic");
    expect(stockErrorKey(undefined)).toBe("services.stock.errors.generic");
  });
});
