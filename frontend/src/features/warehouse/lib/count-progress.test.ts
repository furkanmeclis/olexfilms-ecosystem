import { describe, expect, it } from "vitest";

import {
  countAccuracyPercent,
  countDurationMinutes,
  countProgressPercent,
  countResultBadges,
  countResultTone,
} from "@/features/warehouse/lib/count-progress";
import type { StockCount } from "@/features/warehouse/services/warehouse.service";

const base = {
  progress: {
    scans: 12,
    serial_units: 8,
    fixed_quantity: 2,
    product_quantity: 0,
  },
  expected: { serial_units: 15, fixed_quantity: 5, locations: [] },
  started_at: "2026-10-27T10:00:00Z",
  completed_at: "2026-10-27T10:42:30Z",
  summary: {
    lines: 20,
    unresolved: 3,
    by_result: { matched: 15, missing: 3, wrong_location: 2, nothing: 0 },
  },
} as unknown as StockCount;

describe("count progress (TEC-1032)", () => {
  it("guided: counted over expected, capped at 100", () => {
    expect(countProgressPercent(base)).toBe(50);
    const over = {
      ...base,
      progress: { ...base.progress, serial_units: 30 },
    } as StockCount;
    expect(countProgressPercent(over)).toBe(100);
  });

  it("blind or empty: no percentage", () => {
    expect(
      countProgressPercent({ ...base, expected: undefined } as StockCount),
    ).toBeNull();
    expect(
      countProgressPercent({
        ...base,
        expected: { serial_units: 0, fixed_quantity: 0, locations: [] },
      } as StockCount),
    ).toBeNull();
  });

  it("accuracy, duration and result badges from the summary", () => {
    expect(countAccuracyPercent(base)).toBe(75);
    expect(
      countAccuracyPercent({ ...base, summary: undefined } as StockCount),
    ).toBeNull();
    expect(countDurationMinutes(base)).toBe(43);
    expect(
      countDurationMinutes({ ...base, completed_at: null } as StockCount),
    ).toBeNull();
    expect(countResultBadges(base)).toEqual([
      { result: "matched", count: 15 },
      { result: "missing", count: 3 },
      { result: "wrong_location", count: 2 },
    ]);
    expect(countResultTone("matched")).toBe("success");
    expect(countResultTone("missing")).toBe("danger");
    expect(countResultTone("wrong_location")).toBe("warning");
    expect(countResultTone("other")).toBe("default");
  });
});
