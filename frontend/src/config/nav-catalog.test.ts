import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function catalogLabels(orgType: string) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => x === Permission.CatalogRead),
    canAny: () => false,
    org: { type: orgType, role: "owner", features: ["catalog"] },
  };
  return tenantNav("acme")
    .groups.filter((group) =>
      group.items.some((i) => i.id === "catalog-products"),
    )
    .flatMap((group) => {
      const items = visibleNavItems(group, access);
      if (!items.length) return [];
      return [
        {
          group: group.labelKey,
          products: items.find((i) => i.id === "catalog-products")?.titleKey,
        },
      ];
    });
}

describe("tenant nav: catalog labels (TEC-862)", () => {
  it("drops stock from the dealer labels", () => {
    expect(catalogLabels("dealer")).toEqual([
      {
        group: "layout.section_catalog_dealer",
        products: "layout.nav_catalog_products_dealer",
      },
    ]);
  });

  it("keeps the shared labels for center and distributor", () => {
    for (const type of ["center", "distributor"]) {
      expect(catalogLabels(type)).toEqual([
        {
          group: "layout.section_catalog",
          products: "layout.nav_catalog_products",
        },
      ]);
    }
  });
});

describe("tenant nav: legacy order (TEC-1074)", () => {
  const ids = tenantNav("acme").groups.map((g) => g.id);

  it("puts services right after the home group", () => {
    expect(ids.slice(0, 2)).toEqual(["tenant", "services"]);
  });

  it("keeps the service catalog with the catalog, below customers", () => {
    const catalog = ids.indexOf("catalog");
    expect(ids.indexOf("service-catalog")).toBe(
      ids.indexOf("catalog-dealer") + 1,
    );
    expect(catalog).toBeGreaterThan(ids.indexOf("customers"));
    expect(catalog).toBeGreaterThan(ids.indexOf("orders"));
  });
});
