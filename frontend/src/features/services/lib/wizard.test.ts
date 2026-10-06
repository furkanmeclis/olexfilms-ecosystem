import { describe, expect, it } from "vitest";

import {
  WIZARD_STEPS,
  canOpenStep,
  contractBlocksNext,
  nextStep,
  parseKm,
  previousStep,
  wizardSteps,
} from "./wizard";

describe("wizard steps", () => {
  it("has five steps in order, the contract right before stock", () => {
    expect(WIZARD_STEPS).toEqual([
      "customer_vehicle",
      "parts",
      "measurement",
      "contract",
      "stock",
    ]);
    expect(nextStep("customer_vehicle")).toBe("parts");
    expect(nextStep("measurement")).toBe("contract");
    expect(nextStep("contract")).toBe("stock");
    expect(nextStep("stock")).toBeNull();
    expect(previousStep("measurement")).toBe("parts");
    expect(previousStep("customer_vehicle")).toBeNull();
  });

  it("opens later steps only after the draft exists", () => {
    expect(canOpenStep("customer_vehicle", false)).toBe(true);
    expect(canOpenStep("measurement", false)).toBe(false);
    expect(canOpenStep("measurement", true)).toBe(true);
  });

  it("drops the contract step while the module is off", () => {
    const off = wizardSteps(false);
    expect(off).not.toContain("contract");
    expect(nextStep("measurement", off)).toBe("stock");
    expect(previousStep("stock", off)).toBe("measurement");
    expect(wizardSteps(true)).toContain("contract");
  });

  it("keeps the steps after the contract closed while it blocks", () => {
    expect(canOpenStep("stock", true, true)).toBe(false);
    expect(canOpenStep("contract", true, true)).toBe(true);
    expect(canOpenStep("parts", true, true)).toBe(true);
    expect(canOpenStep("stock", true, true, wizardSteps(false))).toBe(true);
  });

  it("blocks only a required, not executed contract", () => {
    expect(contractBlocksNext({ contract_required: false })).toBe(false);
    expect(contractBlocksNext({ contract_required: true })).toBe(true);
    expect(
      contractBlocksNext({
        contract_required: true,
        contract: { status: "pending" },
      }),
    ).toBe(true);
    expect(
      contractBlocksNext({
        contract_required: true,
        contract: { status: "executed" },
      }),
    ).toBe(false);
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
