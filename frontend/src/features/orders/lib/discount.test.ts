import { describe, expect, it } from "vitest";

import {
  discountValid,
  previewLine,
  toItemInputs,
  validateOrderForm,
  type OrderFormLine,
} from "@/features/orders/lib/form";

describe("order line discounts (TEC-1064)", () => {
  it("validates a typed discount: empty or 0..100 with two decimals", () => {
    for (const ok of ["", "0", "15", "12,5", "20.25", "100"]) {
      expect(discountValid(ok)).toBe(true);
    }
    for (const bad of ["-1", "100.01", "101", "1.234", "x"]) {
      expect(discountValid(bad)).toBe(false);
    }
    const line: OrderFormLine = {
      product_uuid: "p1",
      sku: "S",
      name: "N",
      unit_type: "piece",
      amount: "1",
      discount_pct: "120",
    };
    expect(validateOrderForm([line]).lines.p1).toBe("discount_invalid");
  });

  it("previews the discounted price, a gift, a typed price and no price", () => {
    expect(previewLine({ list: "100", discount: "20", amount: "2" })).toEqual({
      unit: 80,
      total: 160,
    });
    expect(
      previewLine({ list: "33.3333", discount: "20", amount: "3" }),
    ).toEqual({ unit: 26.6666, total: 80 });
    expect(
      previewLine({ list: "100", discount: "20", gift: true, amount: "2" }),
    ).toEqual({ unit: 0, total: 0 });
    expect(
      previewLine({ list: null, discount: "20", manual: "12,5", amount: "2" }),
    ).toEqual({ unit: 12.5, total: 25 });
    expect(previewLine({ list: null, discount: null, amount: "2" })).toEqual({
      unit: null,
      total: null,
    });
  });

  it("sends a typed discount and the gift flag only on a center order", () => {
    const lines: OrderFormLine[] = [
      {
        product_uuid: "p1",
        sku: "A",
        name: "A",
        unit_type: "piece",
        amount: "2",
        unit_price: "",
        discount_pct: "12,5",
      },
      {
        product_uuid: "p2",
        sku: "B",
        name: "B",
        unit_type: "piece",
        amount: "1",
        unit_price: "9",
        discount_pct: "5",
        is_gift: true,
      },
    ];
    expect(toItemInputs(lines, true)).toEqual([
      { product_uuid: "p1", quantity: 2, discount_pct: "12.5" },
      { product_uuid: "p2", quantity: 1, is_gift: true },
    ]);
    expect(toItemInputs(lines)).toEqual([
      { product_uuid: "p1", quantity: 2 },
      { product_uuid: "p2", quantity: 1 },
    ]);
  });
});
