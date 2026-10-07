import { describe, expect, it } from "vitest";

import {
  addLine,
  canVoidSale,
  cartTotals,
  isBelowCost,
  normalizeMoney,
  profitOf,
  type CartLine,
} from "./sales";

const line = (over: Partial<CartLine> = {}): CartLine => ({
  barcode: "OLX-1",
  productUuid: "p-1",
  name: "Film",
  sku: "F-1",
  unitKind: "serial",
  available: 1,
  quantity: 1,
  unitPrice: "1200.00",
  purchasePrice: "800.00",
  currency: "TRY",
  ...over,
});

describe("dealer sales helpers (TEC-348)", () => {
  it("flags a sale price under the purchase price", () => {
    expect(isBelowCost("700.00", "800.00")).toBe(true);
    expect(isBelowCost("800.00", "800.00")).toBe(false);
    expect(isBelowCost("1200", "800.00")).toBe(false);
    expect(isBelowCost(null, "800.00")).toBe(false);
    expect(isBelowCost("700.00", null)).toBe(false);
    expect(profitOf("1200", "800.00")).toBe("400.00");
    expect(profitOf("700.5", "800")).toBe("-99.50");
  });

  it("normalizes typed prices", () => {
    expect(normalizeMoney("12,5")).toBe("12.50");
    expect(normalizeMoney(" 1200 ")).toBe("1200.00");
    expect(normalizeMoney("1.234")).toBeNull();
    expect(normalizeMoney("-1")).toBeNull();
    expect(normalizeMoney("abc")).toBeNull();
  });

  it("adds a barcode only once", () => {
    const first = addLine([], line());
    expect(first.added).toBe(true);
    expect(first.lines).toHaveLength(1);
    const again = addLine(first.lines, line({ unitPrice: "1.00" }));
    expect(again.added).toBe(false);
    expect(again.lines).toBe(first.lines);
    const other = addLine(first.lines, line({ barcode: "OLX-2" }));
    expect(other.lines.map((l) => l.barcode)).toEqual(["OLX-1", "OLX-2"]);
  });

  it("totals the cart and previews the profit", () => {
    const totals = cartTotals([
      line(),
      line({
        barcode: "OLX-2",
        quantity: 2,
        unitPrice: "100",
        purchasePrice: "60",
      }),
      line({ barcode: "OLX-3", unitPrice: "50", purchasePrice: null }),
    ]);
    expect(totals.total).toBe("1450.00");
    expect(totals.profit).toBe("480.00");
    expect(totals.profitIncomplete).toBe(true);
    expect(totals.invalid).toBe(false);
    expect(cartTotals([line({ unitPrice: "x" })]).invalid).toBe(true);
  });

  it("allows a void only on the sale day", () => {
    const now = new Date(2026, 9, 7, 18, 0);
    const today = new Date(2026, 9, 7, 9, 30).toISOString();
    const yesterday = new Date(2026, 9, 6, 23, 59).toISOString();
    expect(canVoidSale({ sold_at: today, voided: false }, now)).toBe(true);
    expect(canVoidSale({ sold_at: yesterday, voided: false }, now)).toBe(false);
    expect(canVoidSale({ sold_at: today, voided: true }, now)).toBe(false);
  });
});
