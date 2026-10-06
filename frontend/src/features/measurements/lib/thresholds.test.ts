// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";

import { describe, expect, it } from "vitest";

import {
  PLACES,
  composePlaceView,
  fillPart,
  phpNumber,
  type PartMapData,
} from "@/features/measurements/lib/part-map";
import {
  Interpretation,
  averagePartInterpretation,
  interpretationColor,
  levelsByPosition,
  type MapReading,
} from "@/features/measurements/lib/thresholds";

// The backend part map package and its PHP-verified golden files are the
// reference of the port (backend/internal/modules/measurements/svg).
const SVG_DIR = resolve(
  __dirname,
  "../../../../../backend/internal/modules/measurements/svg",
);
const read = (...p: string[]) => readFileSync(join(SVG_DIR, ...p), "utf8");

type Fixture = {
  name: string;
  body_type: string;
  parts: string[];
  measurements: MapReading[];
};

const fixture = JSON.parse(read("testdata", "fixture_sedan.json")) as Fixture;
const readings = fixture.measurements;

type Manifest = {
  pointRadius: string;
  assets: {
    part: string;
    place: string;
    kind: string;
    points: { count: number | null; x: number; y: number }[];
  }[];
};

function partMap(bodyType: string): PartMapData {
  const manifest = JSON.parse(
    read("assets", bodyType, "svg_manifest.json"),
  ) as Manifest;
  return {
    point_radius: Number(manifest.pointRadius || 22),
    assets: manifest.assets.map((a) => {
      let svg: string | null = null;
      try {
        svg = read("assets", bodyType, `${a.part}.svg`);
      } catch {
        svg = null;
      }
      return { ...a, points: a.points ?? [], svg };
    }),
  };
}

/**
 * Parsed and re-serialized without whitespace-only text nodes and the
 * default namespace declaration (PHP prints it first, the DOM in place).
 */
function canon(svg: string): string {
  const doc = new DOMParser().parseFromString(svg, "image/svg+xml");
  const walker = doc.createTreeWalker(doc, NodeFilter.SHOW_TEXT);
  const blank: Node[] = [];
  while (walker.nextNode()) {
    if (!walker.currentNode.textContent?.trim()) blank.push(walker.currentNode);
  }
  blank.forEach((n) => n.parentNode?.removeChild(n));
  return new XMLSerializer()
    .serializeToString(doc.documentElement)
    .replaceAll(' xmlns="http://www.w3.org/2000/svg"', "");
}

describe("interpretation colors (backend fill.go)", () => {
  it("maps every level to the backend color", () => {
    expect(interpretationColor(-1)).toBe("#9CA3AF");
    expect(interpretationColor(0)).toBe("#e9df28");
    expect(interpretationColor(1)).toBe("#55d37a");
    expect(interpretationColor(2)).toBe("#deb50a");
    expect(interpretationColor(3)).toBe("#ec4a08");
    expect(interpretationColor(4)).toBe("#af0025");
    expect(interpretationColor(5)).toBe("#af0025");
    expect(interpretationColor(null)).toBe("#9CA3AF");
    expect(interpretationColor(9)).toBe("#9CA3AF");
  });

  it("averages the fixture parts like the backend", () => {
    // TestAverageInterpretationExcludesNullAndDisabled.
    expect(averagePartInterpretation(readings, "HOOD")).toBe(
      Interpretation.SecondLayer,
    );
    expect(averagePartInterpretation(readings, "RIGHT_REAR_DOOR")).toBe(
      Interpretation.ThickPutty,
    );
    expect(averagePartInterpretation(readings, "RIGHT_FRONT_FENDER")).toBe(
      null,
    );
    expect(averagePartInterpretation(readings, "ROOF")).toBe(
      Interpretation.Original,
    );
  });

  it("keeps the last level of a repeated position", () => {
    const levels = levelsByPosition(
      [
        { part_type: "ROOF", position: 2, interpretation: 1 },
        { part_type: "ROOF", position: 1, interpretation: 0 },
        { part_type: "ROOF", position: 2, interpretation: 3 },
      ],
      "ROOF",
    );
    expect([...levels.entries()]).toEqual([
      [2, 3],
      [1, 0],
    ]);
  });

  it("prints numbers like PHP", () => {
    expect(phpNumber(22)).toBe("22");
    expect(phpNumber(1043.5)).toBe("1043.5");
    expect(phpNumber(523.33333333333337)).toBe("523.33333333333");
  });
});

describe("part map port (backend golden SVGs)", () => {
  const map = partMap(fixture.body_type);

  it.each(fixture.parts)("fills part %s like the backend", (part) => {
    const src = map.assets.find((a) => a.part === part)?.svg ?? "";
    const golden = read("testdata", `${fixture.name}_part_${part}.svg`);
    expect(canon(fillPart(src, part, readings))).toBe(canon(golden));
  });

  it.each(PLACES)("composes the %s view like the backend", (place) => {
    const golden = read("testdata", `${fixture.name}_composite_${place}.svg`);
    const got = composePlaceView(map, place, readings);
    expect(got).not.toBeNull();
    expect(canon(got ?? "")).toBe(canon(golden));
  });
});
