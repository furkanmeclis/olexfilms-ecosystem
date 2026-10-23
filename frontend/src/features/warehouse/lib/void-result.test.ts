import { describe, expect, it } from "vitest";

import { voidHeldWarnings } from "@/features/warehouse/lib/void-result";

const t = (key: string, values?: Record<string, string | number>) =>
  `${key}:${values?.n}:${values?.barcodes}`;

describe("voidHeldWarnings (TEC-774)", () => {
  it("groups barcodes held by a transfer or an order", () => {
    expect(
      voidHeldWarnings(
        [
          { barcode: "A1", voided: true },
          { barcode: "A2", voided: false, reason: "in_order" },
          { barcode: "A3", voided: false, reason: "in_transfer" },
          { barcode: "A4", voided: false, reason: "in_transfer" },
          { barcode: "A5", voided: false, reason: "not_reserved" },
        ],
        t,
      ),
    ).toEqual([
      "warehouse.barcodes.void_held_in_transfer:2:A3, A4",
      "warehouse.barcodes.void_held_in_order:1:A2",
    ]);
  });

  it("returns nothing when no barcode is held", () => {
    expect(
      voidHeldWarnings(
        [{ barcode: "A1", voided: false, reason: "not_found" }],
        t,
      ),
    ).toEqual([]);
  });
});
