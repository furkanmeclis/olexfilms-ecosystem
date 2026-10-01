import { describe, expect, it } from "vitest";

import {
  resolveCatalogAccess,
  resolvePricingAccess,
} from "@/features/catalog/lib/access";

const grants = (...slugs: string[]) => {
  const set = new Set(slugs);
  return (p: string) => set.has(p);
};

describe("resolveCatalogAccess", () => {
  it("lets the center with catalog.write write", () => {
    expect(
      resolveCatalogAccess({
        can: grants("catalog.read", "catalog.write"),
        orgType: "center",
      }),
    ).toEqual({ canRead: true, canWrite: true });
  });

  it("keeps distributors and dealers read-only even with catalog.write (K4)", () => {
    for (const orgType of ["distributor", "dealer"]) {
      expect(
        resolveCatalogAccess({
          can: grants("catalog.read", "catalog.write"),
          orgType,
        }),
      ).toEqual({ canRead: true, canWrite: false });
    }
  });

  it("keeps a center role without catalog.write read-only", () => {
    expect(
      resolveCatalogAccess({ can: grants("catalog.read"), orgType: "center" }),
    ).toEqual({ canRead: true, canWrite: false });
  });

  it("hides everything without catalog.read", () => {
    expect(
      resolveCatalogAccess({ can: grants("catalog.write"), orgType: "center" }),
    ).toEqual({ canRead: false, canWrite: false });
  });
});

describe("resolvePricingAccess", () => {
  const all = grants(
    "pricing.purchase.read",
    "pricing.sale.read",
    "pricing.sale.write",
    "pricing.recommended.read",
    "pricing.recommended.write",
  );

  it("center edits list and distributor prices", () => {
    const a = resolvePricingAccess({ can: all, orgType: "center" });
    expect(a).toMatchObject({
      canView: true,
      canWriteList: true,
      canWriteRecommended: true,
      canReadDistributorPrices: true,
      canWriteDistributorPrices: true,
      canWriteDealerPrice: false,
    });
  });

  it("recommended price needs pricing.recommended.write", () => {
    const a = resolvePricingAccess({
      can: grants("pricing.sale.read", "pricing.sale.write"),
      orgType: "center",
    });
    expect(a.canWriteList).toBe(true);
    expect(a.canWriteRecommended).toBe(false);
  });

  it("distributor edits only its dealer price", () => {
    const a = resolvePricingAccess({ can: all, orgType: "distributor" });
    expect(a).toMatchObject({
      canView: true,
      canWriteList: false,
      canWriteRecommended: false,
      canReadDistributorPrices: false,
      canWriteDistributorPrices: false,
      canWriteDealerPrice: true,
    });
  });

  it("dealer only reads", () => {
    const a = resolvePricingAccess({
      can: grants("pricing.purchase.read"),
      orgType: "dealer",
    });
    expect(a).toEqual({
      canView: true,
      canWriteList: false,
      canWriteRecommended: false,
      canReadDistributorPrices: false,
      canWriteDistributorPrices: false,
      canWriteDealerPrice: false,
    });
  });

  it("no pricing grant hides the price card", () => {
    expect(
      resolvePricingAccess({ can: grants(), orgType: "center" }).canView,
    ).toBe(false);
  });
});
