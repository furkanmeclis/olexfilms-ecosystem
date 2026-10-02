import { describe, expect, it } from "vitest";

import {
  WIZARD_STEPS,
  canOpenStep,
  nextStep,
  parseKm,
  previousStep,
} from "./wizard";

describe("wizard steps", () => {
  it("has four steps in order", () => {
    expect(WIZARD_STEPS).toEqual([
      "customer_vehicle",
      "parts",
      "measurement",
      "stock",
    ]);
    expect(nextStep("customer_vehicle")).toBe("parts");
    expect(nextStep("stock")).toBeNull();
    expect(previousStep("measurement")).toBe("parts");
    expect(previousStep("customer_vehicle")).toBeNull();
  });

  it("opens later steps only after the draft exists", () => {
    expect(canOpenStep("customer_vehicle", false)).toBe(true);
    expect(canOpenStep("measurement", false)).toBe(false);
    expect(canOpenStep("measurement", true)).toBe(true);
  });
});

describe("parseKm", () => {
  it("parses empty, valid and invalid values", () => {
    expect(parseKm("")).toBeNull();
    expect(parseKm(" 12500 ")).toBe(12500);
    expect(parseKm("-1")).toBe("invalid");
    expect(parseKm("1.5")).toBe("invalid");
    expect(parseKm("5000001")).toBe("invalid");
  });
});
