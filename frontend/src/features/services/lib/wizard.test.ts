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
  it("has seven steps: products right after the parts, completion last (TEC-1076)", () => {
    expect(WIZARD_STEPS).toEqual([
      "customer_vehicle",
      "photos",
      "parts",
      "stock",
      "measurement",
      "contract",
      "complete",
    ]);
    expect(nextStep("customer_vehicle", wizardSteps(true))).toBe("parts");
    expect(nextStep("customer_vehicle")).toBe("photos");
    expect(nextStep("parts")).toBe("stock");
    expect(nextStep("stock")).toBe("measurement");
    expect(nextStep("measurement")).toBe("contract");
    expect(nextStep("contract")).toBe("complete");
    expect(nextStep("complete")).toBeNull();
    expect(previousStep("measurement")).toBe("stock");
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
    expect(nextStep("measurement", off)).toBe("complete");
    expect(previousStep("complete", off)).toBe("measurement");
    expect(wizardSteps(true)).toContain("contract");
  });

  it("lists photos after customer_vehicle only with the module (TEC-500)", () => {
    expect(wizardSteps(true)).not.toContain("photos");
    expect(wizardSteps(true, false)).not.toContain("photos");
    const on = wizardSteps(true, true);
    expect(on.slice(0, 3)).toEqual(["customer_vehicle", "photos", "parts"]);
    expect(nextStep("customer_vehicle", on)).toBe("photos");
    expect(previousStep("parts", on)).toBe("photos");
  });

  it("keeps the steps after the photos closed while an angle is missing", () => {
    const on = wizardSteps(true, true);
    expect(canOpenStep("photos", true, false, on, true)).toBe(true);
    expect(canOpenStep("parts", true, false, on, true)).toBe(false);
    expect(canOpenStep("contract", true, false, on, true)).toBe(false);
    expect(canOpenStep("stock", true, false, on, true)).toBe(false);
    expect(canOpenStep("parts", true, false, on, false)).toBe(true);
  });

  it("keeps the steps after the contract closed while it blocks", () => {
    expect(canOpenStep("complete", true, true)).toBe(false);
    expect(canOpenStep("contract", true, true)).toBe(true);
    // The products come before the contract, so they stay open.
    expect(canOpenStep("stock", true, true)).toBe(true);
    expect(canOpenStep("parts", true, true)).toBe(true);
    expect(canOpenStep("complete", true, true, wizardSteps(false))).toBe(true);
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
