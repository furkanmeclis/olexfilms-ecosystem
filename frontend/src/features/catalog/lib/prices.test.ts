import { describe, expect, it } from "vitest";

import {
  parseLines,
  productFormSchema,
  productInput,
  productDefaults,
} from "@/features/catalog/lib/form";
import {
  computedMeterPrice,
  listPriceBody,
  normalizePriceInput,
  visiblePriceColumns,
} from "@/features/catalog/lib/prices";
import { resolveSearchHitHref } from "@/features/search-engine/lib/hit-href";

describe("visiblePriceColumns", () => {
  it("dealer sees only its purchase price, even if a sale field slips through", () => {
    const cols = visiblePriceColumns("dealer", [
      {
        currency: "TRY",
        purchase_price: "10.00",
        purchase_price_source: "distributor",
        sale_price: "99.00",
      },
    ]);
    expect(cols.map((c) => c.field)).toEqual(["purchase_price"]);
  });

  it("distributor sees purchase and its dealer price, never recommended", () => {
    const cols = visiblePriceColumns("distributor", [
      {
        currency: "TRY",
        purchase_price: "10.00",
        sale_price: "12.00",
        recommended_sale_price: "20.00",
      },
    ]);
    expect(cols.map((c) => c.field)).toEqual(["purchase_price", "sale_price"]);
  });

  it("center columns follow the masked answer", () => {
    const cols = visiblePriceColumns("center", [
      { currency: "TRY", sale_price: "12.00" },
      { currency: "EUR", sale_price: "1.00" },
    ]);
    expect(cols.map((c) => c.field)).toEqual(["sale_price"]);
  });
});

describe("listPriceBody", () => {
  it("sends filled fields, clears emptied ones and leaves masked ones out", () => {
    const body = listPriceBody(
      {
        currency: "TRY",
        purchase_price: "",
        sale_to_distributor_price: "15.5",
        recommended_sale_price: "",
      },
      { currency: "TRY", sale_price: "12.00", recommended_sale_price: "20" },
      true,
    );
    expect(body).toEqual({
      sale_to_distributor_price: "15.5",
      recommended_sale_price: null,
    });
  });

  it("never touches the recommended price without its write grant", () => {
    const body = listPriceBody(
      { recommended_sale_price: "30", purchase_price: "8" },
      null,
      false,
    );
    expect(body).toEqual({ purchase_price: "8" });
  });

  it("normalizes a decimal comma", () => {
    expect(normalizePriceInput(" 12,50 ")).toBe("12.50");
  });
});

describe("product form", () => {
  it("parses one entry per line without blanks or duplicates", () => {
    expect(parseLines("Kaput\n\n Tavan \nKaput\r\nÇamurluk")).toEqual([
      "Kaput",
      "Tavan",
      "Çamurluk",
    ]);
  });

  it("maps values to the API body (empty numbers clear, images untouched)", () => {
    const values = {
      ...productDefaults(null),
      category_uuid: "c1",
      sku: " PPF-1 ",
      name: "PPF",
      unit_type: "roll_meter" as const,
      micron_thickness: "190,5",
    };
    const body = productInput(values);
    expect(body).not.toHaveProperty("images");
    expect(body).toMatchObject({
      sku: "PPF-1",
      unit_type: "roll_meter",
      warranty_duration_months: null,
      micron_thickness: 190.5,
      uses_fixed_barcode: false,
      active: true,
      usage_credits: 5,
    });
  });

  it("keeps the usage credits of a product within 1..1000 (TEC-1221)", () => {
    const schema = productFormSchema((key) => key);
    const values = {
      ...productDefaults(null),
      category_uuid: "c1",
      sku: "S",
      name: "N",
    };
    expect(values.usage_credits).toBe("5");
    expect(schema.safeParse(values).success).toBe(true);
    expect(
      productInput({ ...values, usage_credits: " 12 " }).usage_credits,
    ).toBe(12);
    for (const bad of ["0", "1001", "2.5", ""]) {
      expect(schema.safeParse({ ...values, usage_credits: bad }).success).toBe(
        false,
      );
    }
  });
});

describe("resolveSearchHitHref", () => {
  it("prefixes catalog hits with the tenant shell", () => {
    expect(resolveSearchHitHref("/catalog/products/u1", "merkez")).toBe(
      "/t/merkez/catalog/products/u1",
    );
  });

  it("keeps platform hits and hits outside a tenant", () => {
    expect(resolveSearchHitHref("/platform/users/u1", "merkez")).toBe(
      "/platform/users/u1",
    );
    expect(resolveSearchHitHref("/catalog/products/u1", null)).toBe(
      "/catalog/products/u1",
    );
  });
});

// TEC-1238 (K2): the meter list price.
describe("meter price", () => {
  it("previews box price ÷ box meters", () => {
    expect(computedMeterPrice("400", 15)).toBe(26.6667);
    expect(computedMeterPrice("400", null)).toBeNull();
    expect(computedMeterPrice(undefined, 15)).toBeNull();
  });

  it("sends meter_price only when the dialog shows it", () => {
    expect(
      listPriceBody({ sale_to_distributor_price: "400" }, null, false),
    ).toEqual({
      sale_to_distributor_price: "400",
    });
    expect(
      listPriceBody(
        { sale_to_distributor_price: "400", meter_price: "30" },
        null,
        false,
      ),
    ).toEqual({ sale_to_distributor_price: "400", meter_price: "30" });
    // Emptied after a stored value: cleared (computed again).
    expect(
      listPriceBody(
        { sale_to_distributor_price: "400", meter_price: "" },
        { currency: "USD", sale_price: "400", meter_price: "30" },
        false,
      ),
    ).toEqual({ sale_to_distributor_price: "400", meter_price: null });
  });

  it("shows the meter list price column to the center only", () => {
    const row = { currency: "USD", sale_price: "400", meter_price: "30" };
    expect(visiblePriceColumns("center", [row]).map((c) => c.field)).toContain(
      "meter_price",
    );
    expect(
      visiblePriceColumns("dealer", [row]).map((c) => c.field),
    ).not.toContain("meter_price");
  });
});
