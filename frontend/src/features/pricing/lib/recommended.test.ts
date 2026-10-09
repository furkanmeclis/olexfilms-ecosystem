import { describe, expect, it } from "vitest";

import {
  addDays,
  applyPercent,
  effectiveFromError,
  isOverThreshold,
  normalizePublishPrice,
  publishRows,
  recommendedCsv,
} from "./recommended";

describe("recommended price helpers (TEC-507)", () => {
  it("flags |deviation| >= threshold either way", () => {
    expect(isOverThreshold("15.00", 15)).toBe(true);
    expect(isOverThreshold("-15.01", 15)).toBe(true);
    expect(isOverThreshold("14.99", 15)).toBe(false);
    expect(isOverThreshold(null, 15)).toBe(false);
    expect(isOverThreshold("x", 15)).toBe(false);
  });

  it("refuses a past or malformed effective day", () => {
    expect(effectiveFromError("2026-10-08", "2026-10-09")).toBe("past");
    expect(effectiveFromError("2026-10-09", "2026-10-09")).toBeNull();
    expect(effectiveFromError("2027-01-01", "2026-10-09")).toBeNull();
    expect(effectiveFromError("", "2026-10-09")).toBe("required");
    expect(effectiveFromError("09.10.2026", "2026-10-09")).toBe("invalid");
  });

  it("normalizes prices to 2 decimals", () => {
    expect(normalizePublishPrice("1250,5")).toBe("1250.50");
    expect(normalizePublishPrice("1.250,50")).toBe("1250.50");
    expect(normalizePublishPrice(" 99 ")).toBe("99.00");
    expect(normalizePublishPrice("-5")).toBeNull();
    expect(normalizePublishPrice("abc")).toBeNull();
    expect(normalizePublishPrice("")).toBeNull();
  });

  it("applies a percent change", () => {
    expect(applyPercent("1000.00", 10)).toBe("1100.00");
    expect(applyPercent("999.99", -5)).toBe("949.99");
    expect(applyPercent("100", -150)).toBeNull();
  });

  it("adds days across month ends", () => {
    expect(addDays("2026-10-31", 1)).toBe("2026-11-01");
  });

  it("maps the basket to publish rows and the CSV to import columns", () => {
    const rows = [
      {
        productUuid: "p1",
        productName: "Film",
        productSku: "F,1",
        country: "",
        currency: "EUR",
        price: "10.00",
        previous: null,
      },
    ];
    expect(publishRows(rows)).toEqual([
      { product_uuid: "p1", country: null, currency: "EUR", price: "10.00" },
    ]);
    expect(
      recommendedCsv([
        { sku: "F,1", country: "", currency: "EUR", price: "10.00" },
      ]),
    ).toBe('sku,country,currency,price,effective_from\n"F,1",,EUR,10.00,\n');
  });
});
