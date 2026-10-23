import { describe, expect, it } from "vitest";

import {
  BODY_PARTS,
  PART_PRESETS,
  WINDOW_PARTS,
  applyPreset,
  isKnownPart,
  isPresetActive,
  partGroup,
  selectionMatchesPresets,
  visiblePresets,
} from "./car-parts";

const preset = (id: string) => PART_PRESETS.find((p) => p.id === id)!;
const sorted = (xs: readonly string[]) => [...xs].sort();
const ALL = [...BODY_PARTS, ...WINDOW_PARTS];

describe("part presets (TEC-864)", () => {
  it("defines the body and window presets in order", () => {
    expect(
      PART_PRESETS.filter((p) => p.group === "body").map((p) => p.id),
    ).toEqual([
      "hood_only",
      "front_three",
      "front_four",
      "full_body",
      "full_body_no_bumpers",
      "full_body_no_roof",
    ]);
    expect(
      PART_PRESETS.filter((p) => p.group === "window").map((p) => p.id),
    ).toEqual([
      "windshield",
      "sunroof",
      "front_two_windows",
      "all_windows",
      "all_windows_no_windshield",
      "all_windows_no_roof",
    ]);
  });

  it("only uses known parts of the preset's own group", () => {
    for (const p of PART_PRESETS) {
      expect(p.parts.length).toBeGreaterThan(0);
      expect(new Set(p.parts).size).toBe(p.parts.length);
      for (const part of p.parts) {
        expect(isKnownPart(part)).toBe(true);
        expect(partGroup(part)).toBe(p.group);
      }
    }
  });

  it("maps each preset to the agreed parts", () => {
    expect(preset("hood_only").parts).toEqual(["body_kaput"]);
    expect(sorted(preset("front_three").parts)).toEqual(
      sorted(["body_kaput", "body_sol_on_camurluk", "body_sag_on_camurluk"]),
    );
    expect(sorted(preset("front_four").parts)).toEqual(
      sorted([
        "body_kaput",
        "body_on_tampon",
        "body_sol_on_camurluk",
        "body_sag_on_camurluk",
      ]),
    );
    expect(sorted(preset("full_body").parts)).toEqual(sorted(BODY_PARTS));
    expect(sorted(preset("full_body_no_bumpers").parts)).toEqual(
      sorted(
        BODY_PARTS.filter(
          (p) => p !== "body_on_tampon" && p !== "body_arka_tampon",
        ),
      ),
    );
    expect(sorted(preset("full_body_no_roof").parts)).toEqual(
      sorted(BODY_PARTS.filter((p) => p !== "body_tavan")),
    );
    expect(preset("windshield").parts).toEqual(["window_on_cam"]);
    expect(preset("sunroof").parts).toEqual(["window_sunroof"]);
    expect(sorted(preset("front_two_windows").parts)).toEqual(
      sorted(["window_sol_on_kapi", "window_sag_on_kapi"]),
    );
    expect(sorted(preset("all_windows").parts)).toEqual(sorted(WINDOW_PARTS));
    expect(sorted(preset("all_windows_no_windshield").parts)).toEqual(
      sorted(WINDOW_PARTS.filter((p) => p !== "window_on_cam")),
    );
    expect(sorted(preset("all_windows_no_roof").parts)).toEqual(
      sorted(WINDOW_PARTS.filter((p) => p !== "window_sunroof")),
    );
  });

  it("replaces only its own group and skips unavailable parts", () => {
    const available = BODY_PARTS.filter((p) => p !== "body_tavan");
    const next = applyPreset(
      ["body_kaput", "window_on_cam"],
      preset("full_body"),
      [...available, "window_on_cam"],
    );
    expect(sorted(next)).toEqual(sorted([...available, "window_on_cam"]));
  });

  it("shows only presets with available parts and drops duplicates", () => {
    expect(visiblePresets(ALL).map((p) => p.id)).toEqual(
      PART_PRESETS.map((p) => p.id),
    );
    // Window film category: no body presets.
    expect(
      visiblePresets(WINDOW_PARTS).every((p) => p.group === "window"),
    ).toBe(true);
    // No sunroof: "all windows except roof" equals "all windows".
    const noRoof = WINDOW_PARTS.filter((p) => p !== "window_sunroof");
    expect(visiblePresets(noRoof).map((p) => p.id)).toEqual([
      "windshield",
      "front_two_windows",
      "all_windows",
      "all_windows_no_windshield",
    ]);
    expect(visiblePresets([])).toEqual([]);
  });

  it("tells whether the selection is explained by the presets", () => {
    const presets = visiblePresets(ALL);
    expect(selectionMatchesPresets([], presets, ALL)).toBe(true);
    const front = applyPreset([], preset("front_three"), ALL);
    const both = applyPreset(front, preset("windshield"), ALL);
    expect(selectionMatchesPresets(both, presets, ALL)).toBe(true);
    expect(isPresetActive(both, preset("front_three"), ALL)).toBe(true);
    expect(
      selectionMatchesPresets(["body_kaput", "body_tavan"], presets, ALL),
    ).toBe(false);
    expect(selectionMatchesPresets(["custom_part"], presets, ALL)).toBe(false);
  });
});
