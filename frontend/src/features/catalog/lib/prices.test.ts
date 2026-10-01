import { describe, expect, it } from "vitest";

import {
  parseLines,
  productInput,
  productDefaults,
} from "@/features/catalog/lib/form";
import {
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

  it("maps values to the API body (empty numbers clear, images keep order)", () => {
    const values = {
      ...productDefaults(null),
      category_uuid: "c1",
      sku: " PPF-1 ",
      name: "PPF",
      unit_type: "roll_meter" as const,
      micron_thickness: "190,5",
      images: "a.jpg\nb.jpg",
    };
    expect(productInput(values)).toMatchObject({
      sku: "PPF-1",
      unit_type: "roll_meter",
      warranty_duration_months: null,
      micron_thickness: 190.5,
      images: [
        { key: "a.jpg", sort: 0 },
        { key: "b.jpg", sort: 1 },
      ],
      uses_fixed_barcode: false,
      active: true,
    });
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
