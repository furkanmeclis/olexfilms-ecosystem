import { describe, expect, it } from "vitest";

import {
  canRequestReclassification,
  canSplitRoll,
  movementDocumentHref,
  reclassActions,
  reclassFormErrors,
  splitPreview,
  toCents,
  type OperableUnit,
} from "./stock-ops";

const roll: OperableUnit = {
  barcode: "OLX-ROLL-1",
  status: "available",
  remaining_meters: "37.50",
  product: {
    uuid: "p-1",
    name: "PPF 190",
    sku: "PPF-190",
    unit_type: "roll_meter",
    uses_fixed_barcode: false,
  },
};

describe("unit operation gates (TEC-763)", () => {
  it("splits only a roll on hand with meters left", () => {
    expect(canSplitRoll(roll)).toBe(true);
    expect(canSplitRoll({ ...roll, status: "placed" })).toBe(true);
    expect(canSplitRoll({ ...roll, status: "used" })).toBe(false);
    expect(canSplitRoll({ ...roll, status: "in_transit" })).toBe(false);
    expect(canSplitRoll({ ...roll, remaining_meters: "0.00" })).toBe(false);
    expect(
      canSplitRoll({
        ...roll,
        product: { ...roll.product!, unit_type: "piece" },
      }),
    ).toBe(false);
  });

  it("requests a product fix for a serial unit on hand only", () => {
    expect(canRequestReclassification(roll)).toBe(true);
    expect(canRequestReclassification({ ...roll, status: "void" })).toBe(false);
    expect(
      canRequestReclassification({
        ...roll,
        product: { ...roll.product!, uses_fixed_barcode: true },
      }),
    ).toBe(false);
    expect(canRequestReclassification({ ...roll, product: null })).toBe(false);
  });
});

describe("splitPreview (TEC-763)", () => {
  it("parses meters in cents", () => {
    expect(toCents("12")).toBe(1200);
    expect(toCents("12.5")).toBe(1250);
    expect(toCents("12,05")).toBe(1205);
    expect(toCents("1.234")).toBeNaN();
    expect(toCents("-1")).toBeNaN();
  });

  it("previews what stays on the roll", () => {
    expect(splitPreview("37.50", "12.5")).toEqual({
      error: null,
      meters: "12.50",
      left: "25.00",
    });
    expect(splitPreview("37.50", "37.50")).toEqual({
      error: null,
      meters: "37.50",
      left: "0.00",
    });
  });

  it("rejects empty, malformed, zero and too many meters", () => {
    expect(splitPreview("37.50", " ").error).toBe("required");
    expect(splitPreview("37.50", "abc").error).toBe("format");
    expect(splitPreview("37.50", "1.234").error).toBe("format");
    expect(splitPreview("37.50", "0").error).toBe("positive");
    expect(splitPreview("37.50", "37.51").error).toBe("exceeds");
  });
});

describe("reclassFormErrors (TEC-763)", () => {
  it("needs a target and a reason", () => {
    expect(reclassFormErrors({ target: "", reason: "  " })).toEqual({
      target: "stock.reclass.errors.target_required",
      reason: "stock.reclass.errors.reason_required",
    });
    expect(
      reclassFormErrors({ target: "p-2", reason: "x".repeat(2001) }),
    ).toEqual({ reason: "stock.reclass.errors.reason_too_long" });
    expect(reclassFormErrors({ target: "p-2", reason: "wrong label" })).toEqual(
      {},
    );
  });
});

describe("reclassActions (TEC-157 four eyes)", () => {
  const row = {
    status: "pending" as const,
    requested_by: { uuid: "user-a", name: "A" },
    organization: {
      uuid: "org-dealer",
      name: "Dealer",
      type: "dealer" as const,
    },
  };
  const center = {
    userUuid: "user-b",
    organizationUuid: "org-center",
    canReclassify: true,
    canWrite: true,
  };

  it("lets another center user approve and reject", () => {
    expect(reclassActions(row, center)).toEqual({
      approve: true,
      reject: true,
      cancel: false,
      selfRequested: false,
    });
  });

  it("never offers the requester the approval", () => {
    const self = reclassActions(
      { ...row, organization: { ...row.organization, uuid: "org-center" } },
      { ...center, userUuid: "user-a" },
    );
    expect(self.approve).toBe(false);
    expect(self.selfRequested).toBe(true);
    expect(self.reject).toBe(true);
    expect(self.cancel).toBe(true);
  });

  it("needs stock.reclassify to decide and stock.write in the org to cancel", () => {
    const dealer = reclassActions(row, {
      userUuid: "user-a",
      organizationUuid: "org-dealer",
      canReclassify: false,
      canWrite: true,
    });
    expect(dealer).toEqual({
      approve: false,
      reject: false,
      cancel: true,
      selfRequested: false,
    });
    expect(
      reclassActions(row, {
        ...center,
        organizationUuid: "org-dealer",
        canWrite: false,
      }).cancel,
    ).toBe(false);
  });

  it("decides pending requests only", () => {
    expect(reclassActions({ ...row, status: "approved" }, center)).toEqual({
      approve: false,
      reject: false,
      cancel: false,
      selfRequested: false,
    });
  });
});

describe("movementDocumentHref (TEC-763)", () => {
  it("links each document type to its page", () => {
    expect(
      movementDocumentHref("acme", { type: "stock_entry", uuid: "e-1" }),
    ).toBe("/t/acme/warehouse/entries/e-1");
    expect(movementDocumentHref("acme", { type: "service", uuid: "s-1" })).toBe(
      "/t/acme/services/s-1",
    );
    expect(
      movementDocumentHref("acme", {
        type: "stock_reclassification",
        uuid: "r-1",
      }),
    ).toBe("/t/acme/stock/reclassifications/r-1");
  });
});
