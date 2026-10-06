import { describe, expect, it } from "vitest";

import {
  parseDecimal,
  showPurchasePrice,
  summarizeProducts,
  unitStatusTone,
} from "@/features/stock/lib/stock";
import type {
  StockProduct,
  StockUnitRow,
} from "@/features/stock/services/stock.service";

function unit(patch: Partial<StockUnitRow> = {}): StockUnitRow {
  return {
    uuid: "u-1",
    barcode: "OLX-1",
    unit_kind: "serial",
    status: "available",
    quantity: 1,
    initial_meters: null,
    remaining_meters: null,
    product: {
      uuid: "p-1",
      sku: "P1",
      name: "Film",
      unit_type: "piece",
      uses_fixed_barcode: false,
    },
    location: null,
    purchase_price: null,
    updated_at: "2026-10-02T10:00:00Z",
    ...patch,
  };
}

function product(patch: Partial<StockProduct> = {}): StockProduct {
  return {
    product: {
      uuid: "p-1",
      sku: "P1",
      name: "Film",
      unit_type: "roll_meter",
      uses_fixed_barcode: false,
      active: true,
      category: { uuid: "c-1", name: "PPF" },
    },
    quantity: 0,
    meters: "0.00",
    fixed_barcodes: [],
    updated_at: "2026-10-02T10:00:00Z",
    ...patch,
  };
}

describe("showPurchasePrice", () => {
  it("is false when every row has a null price", () => {
    expect(showPurchasePrice([unit(), unit({ uuid: "u-2" })])).toBe(false);
    expect(showPurchasePrice([])).toBe(false);
  });

  it("is true when one row carries a price", () => {
    expect(
      showPurchasePrice([
        unit(),
        unit({
          uuid: "u-2",
          purchase_price: { amount: "10.00", currency: "TRY", source: "list" },
        }),
      ]),
    ).toBe(true);
  });
});

describe("summarizeProducts", () => {
  it("sums pieces and meters across products", () => {
    expect(
      summarizeProducts([
        product({ quantity: 3, meters: "12.50" }),
        product({
          product: { ...product().product, uuid: "p-2", unit_type: "piece" },
          quantity: 7,
          meters: "0.00",
        }),
        product({
          product: { ...product().product, uuid: "p-3" },
          quantity: 0,
          meters: "0.10",
        }),
      ]),
    ).toEqual({ products: 3, quantity: 10, meters: 12.6 });
  });

  it("is all zero for an empty list", () => {
    expect(summarizeProducts([])).toEqual({
      products: 0,
      quantity: 0,
      meters: 0,
    });
  });
});

describe("parseDecimal / unitStatusTone", () => {
  it("reads decimal strings and treats garbage as zero", () => {
    expect(parseDecimal("12.50")).toBe(12.5);
    expect(parseDecimal(null)).toBe(0);
    expect(parseDecimal("abc")).toBe(0);
  });

  it("maps statuses to chip tones", () => {
    expect(unitStatusTone("available")).toBe("success");
    expect(unitStatusTone("placed")).toBe("warning");
    expect(unitStatusTone("used")).toBe("danger");
    expect(unitStatusTone("printed")).toBe("default");
  });
});
