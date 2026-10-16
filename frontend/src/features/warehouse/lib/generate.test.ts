import { describe, expect, it } from "vitest";

import {
  defaultLevels,
  fixLevelChain,
  GENERATE_CONFIRM_THRESHOLD,
  levelCodes,
  newLevel,
  planGeneration,
  toGenerateLevel,
  type LevelDraft,
} from "@/features/warehouse/lib/generate";
import { buildLocationTree } from "@/features/warehouse/lib/tree";
import type { WarehouseLocation } from "@/features/warehouse/services/warehouse.service";

const range = (
  type: LevelDraft["type"],
  from: number,
  to: number,
  pad = 0,
  prefix = "",
): LevelDraft => ({
  type,
  mode: "range",
  prefix,
  from: String(from),
  to: String(to),
  pad: String(pad),
  codes: "",
});
const list = (type: LevelDraft["type"], codes: string): LevelDraft => ({
  ...newLevel(type),
  mode: "list",
  codes,
});

function loc(
  uuid: string,
  type: WarehouseLocation["type"],
  code: string,
  parent: WarehouseLocation | null,
): WarehouseLocation {
  return {
    uuid,
    warehouse_uuid: "w1",
    room_uuid: "r1",
    parent_uuid: parent?.uuid ?? null,
    type,
    code,
    full_code: `${parent ? parent.full_code : "WH1-R1"}-${code}`,
    name: code,
    active: true,
    sort_order: 0,
    created_at: "2026-10-01T09:00:00Z",
    updated_at: "2026-10-01T09:00:00Z",
  };
}

describe("levelCodes (TEC-764)", () => {
  it("renders a range as prefix + zero-padded number like the backend", () => {
    expect(levelCodes(range("shelf", 1, 3, 2, "s"))).toEqual({
      ok: true,
      codes: ["S01", "S02", "S03"],
    });
    expect(levelCodes(range("bin", 9, 11))).toEqual({
      ok: true,
      codes: ["9", "10", "11"],
    });
  });

  it("splits and upper-cases a code list", () => {
    expect(levelCodes(list("aisle", " a, b;c\nd "))).toEqual({
      ok: true,
      codes: ["A", "B", "C", "D"],
    });
  });

  it("refuses bad ranges, pads, codes and repeats", () => {
    expect(levelCodes(range("bin", 5, 1))).toMatchObject({
      error: { key: "range" },
    });
    expect(levelCodes({ ...range("bin", 1, 2), to: "x" })).toMatchObject({
      error: { key: "range" },
    });
    expect(levelCodes(range("bin", 1, 10000))).toMatchObject({
      error: { key: "range" },
    });
    expect(levelCodes(range("bin", 1, 2, 5))).toMatchObject({
      error: { key: "pad" },
    });
    expect(levelCodes(list("aisle", ""))).toMatchObject({
      error: { key: "codes_required" },
    });
    expect(levelCodes(list("aisle", "A, B-1"))).toEqual({
      ok: false,
      error: { key: "code_invalid", code: "B-1" },
    });
    expect(levelCodes(list("aisle", "A, a"))).toEqual({
      ok: false,
      error: { key: "duplicate", code: "A" },
    });
  });

  it("builds the request level", () => {
    expect(toGenerateLevel(range("shelf", 1, 10, 2, "s"))).toEqual({
      type: "shelf",
      from: 1,
      to: 10,
      pad: 2,
      prefix: "S",
    });
    expect(toGenerateLevel(list("aisle", "a b"))).toEqual({
      type: "aisle",
      codes: ["A", "B"],
    });
  });
});

describe("level chain", () => {
  it("defaults to aisle › shelf › bin at the root and follows the parent", () => {
    expect(defaultLevels(null).map((l) => l.type)).toEqual([
      "aisle",
      "shelf",
      "bin",
    ]);
    expect(defaultLevels("aisle").map((l) => l.type)).toEqual(["shelf", "bin"]);
    expect(defaultLevels("shelf").map((l) => l.type)).toEqual(["bin"]);
    expect(defaultLevels("bin")).toEqual([]);
  });

  it("cuts the chain where a type stops fitting", () => {
    const levels = [newLevel("shelf"), newLevel("bin"), newLevel("bin")];
    expect(fixLevelChain(null, levels).map((l) => l.type)).toEqual([
      "shelf",
      "bin",
    ]);
    expect(fixLevelChain("aisle", [newLevel("bin")])).toEqual([]);
  });
});

describe("planGeneration (preview)", () => {
  it("counts every node and samples the leaf codes", () => {
    const plan = planGeneration({
      levels: [
        list("aisle", "A, B"),
        range("shelf", 1, 5, 2),
        range("bin", 1, 4, 2),
      ],
      parent: null,
      baseCode: "WH1-R1",
      roots: [],
    });
    // 2 aisles + 10 shelves + 40 bins.
    expect(plan.total).toBe(52);
    expect(plan.leaves).toBe(40);
    expect(plan.created).toBe(52);
    expect(plan.existing).toBe(0);
    expect(plan.ok).toBe(true);
    expect(plan.needsExtraConfirm).toBe(false);
    expect(plan.sample).toEqual([
      "WH1-R1-A-01-01",
      "WH1-R1-A-01-02",
      "WH1-R1-A-01-03",
      "WH1-R1-A-01-04",
      "WH1-R1-A-02-01",
      "WH1-R1-A-02-02",
      "WH1-R1-B-05-04",
    ]);
  });

  it("reuses codes that exist with the same type and adds under them", () => {
    const a = loc("a", "aisle", "A", null);
    const s1 = loc("s1", "shelf", "01", a);
    const roots = buildLocationTree([a, s1]);
    const plan = planGeneration({
      levels: [list("aisle", "A, B"), range("shelf", 1, 2, 2)],
      parent: null,
      baseCode: "WH1-R1",
      roots,
    });
    // A and A-01 exist; A-02, B, B-01, B-02 are new.
    expect(plan.existing).toBe(2);
    expect(plan.created).toBe(4);
    expect(plan.conflicts).toEqual([]);
    expect(plan.ok).toBe(true);
  });

  it("flags a code that exists with another type", () => {
    const s = loc("s", "shelf", "A", null);
    const plan = planGeneration({
      levels: [list("aisle", "A"), range("shelf", 1, 1)],
      parent: null,
      baseCode: "WH1-R1",
      roots: buildLocationTree([s]),
    });
    expect(plan.conflicts).toEqual([
      { full_code: "WH1-R1-A", existing: "shelf", wanted: "aisle" },
    ]);
    expect(plan.ok).toBe(false);
  });

  it("generates under a parent location", () => {
    const a = loc("a", "aisle", "A", null);
    const [aisle] = buildLocationTree([a]);
    const plan = planGeneration({
      levels: [range("shelf", 1, 2), range("bin", 1, 3)],
      parent: aisle,
      baseCode: aisle.full_code,
      roots: [aisle],
    });
    expect(plan.total).toBe(8);
    expect(plan.sample[0]).toBe("WH1-R1-A-1-1");
    expect(plan.ok).toBe(true);
  });

  it("asks for an extra confirmation above the threshold and stops at the cap", () => {
    const big = planGeneration({
      levels: [range("shelf", 1, 30), range("bin", 1, 20)],
      parent: null,
      baseCode: "WH1-R1",
      roots: [],
    });
    expect(big.created).toBe(630);
    expect(big.created).toBeGreaterThan(GENERATE_CONFIRM_THRESHOLD);
    expect(big.needsExtraConfirm).toBe(true);

    const huge = planGeneration({
      levels: [range("shelf", 1, 100), range("bin", 1, 20)],
      parent: null,
      baseCode: "WH1-R1",
      roots: [],
    });
    expect(huge.total).toBe(2100);
    expect(huge.tooMany).toBe(true);
    expect(huge.ok).toBe(false);
    expect(huge.sample).toEqual([]);
  });

  it("reports invalid levels and a type that does not fit", () => {
    const bad = planGeneration({
      levels: [list("aisle", ""), range("shelf", 1, 2)],
      parent: null,
      baseCode: "WH1-R1",
      roots: [],
    });
    expect(bad.levelErrors[0]).toEqual({ key: "codes_required" });
    expect(bad.ok).toBe(false);

    const wrong = planGeneration({
      levels: [range("bin", 1, 2)],
      parent: null,
      baseCode: "WH1-R1",
      roots: [],
    });
    expect(wrong.levelErrors[0]).toEqual({ key: "type" });
    expect(wrong.ok).toBe(false);
  });
});
