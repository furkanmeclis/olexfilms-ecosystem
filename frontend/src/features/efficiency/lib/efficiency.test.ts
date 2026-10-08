import { describe, expect, it } from "vitest";

import {
  applyExpectationEdit,
  efficiencyDimensions,
  efficiencyPeriod,
  expectationsCsv,
  formatWaste,
  isAboveWarning,
  percentRangeParams,
  trendTotals,
} from "@/features/efficiency/lib/efficiency";
import type { PartConsumptionExpectation } from "@/features/efficiency/services/efficiency.service";

const percent = (v: number) => `${Math.round(v * 1000) / 10}%`;

const row: PartConsumptionExpectation = {
  uuid: "e1",
  product_uuid: "p1",
  category_uuid: null,
  product_name: "Film, A",
  category_name: null,
  body_type: "sedan",
  part_key: "body_kaput",
  expected_meters: "2.00",
  source: "network",
  sample_size: 24,
};

describe("efficiency lib", () => {
  it('formats a missing waste ratio as "—"', () => {
    expect(formatWaste(null, percent)).toBe("—");
    expect(formatWaste("", percent)).toBe("—");
    expect(formatWaste("0.125", percent)).toBe("12.5%");
  });

  it("flags only ratios strictly above the threshold", () => {
    expect(isAboveWarning("0.16", 0.15)).toBe(true);
    expect(isAboveWarning("0.15", 0.15)).toBe(false);
    expect(isAboveWarning(null, 0.15)).toBe(false);
  });

  it("drops the dealer and staff breakdowns for dealers", () => {
    expect(efficiencyDimensions("dealer")).toEqual([
      "product",
      "body_type",
      "part",
    ]);
    expect(efficiencyDimensions("distributor")).toContain("dealer");
    expect(efficiencyDimensions("center")).toContain("staff");
  });

  it("maps percent filters to ratio params", () => {
    expect(percentRangeParams("waste_ratio")([10, undefined])).toEqual({
      waste_ratio_min: "0.1",
      waste_ratio_max: undefined,
    });
  });

  it("builds an inclusive day period", () => {
    expect(efficiencyPeriod(30, new Date("2026-10-09T12:00:00Z"))).toEqual({
      period_from: "2026-09-09",
      period_to: "2026-10-09",
    });
  });

  it("weights the period waste by services", () => {
    const totals = trendTotals([
      {
        month: "2026-08",
        services: 1,
        meters: "5",
        expected_meters: "5",
        waste_ratio: "0.40",
      },
      {
        month: "2026-09",
        services: 3,
        meters: "10",
        expected_meters: null,
        waste_ratio: null,
      },
      {
        month: "2026-10",
        services: 3,
        meters: "15",
        expected_meters: "14",
        waste_ratio: "0.00",
      },
    ]);
    expect(totals.services).toBe(7);
    expect(totals.meters).toBe(30);
    expect(totals.wasteRatio).toBeCloseTo(0.1);
  });

  it("returns a network row to manual on edit", () => {
    expect(applyExpectationEdit(row, "expected_meters", "2,4")).toMatchObject({
      expected_meters: "2.40",
      source: "manual",
    });
    expect(applyExpectationEdit(row, "body_type", " ")).toMatchObject({
      body_type: null,
      source: "manual",
    });
    expect(applyExpectationEdit(row, "expected_meters", "-1")).toBeNull();
    expect(applyExpectationEdit(row, "part_key", "x")).toBeNull();
  });

  it("quotes CSV cells and keeps the import headers", () => {
    const csv = expectationsCsv([row]).split("\n");
    expect(csv[0]).toBe(
      "uuid,product,category,body_type,part_key,expected_meters,source,sample_size",
    );
    expect(csv[1]).toBe('e1,"Film, A",,sedan,body_kaput,2.00,network,24');
  });
});
