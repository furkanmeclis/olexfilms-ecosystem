import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

const DEALER_SALES_ITEMS = [
  "dealer-sales-quick-sale",
  "dealer-sales-prices",
  "dealer-sales-suppliers",
  "dealer-sales-purchases",
];

/** dealer_owner / dealer_accounting hold these (backend rbac catalog). */
const DEALER_SALES_GRANTS = [
  Permission.AccountingRead,
  Permission.DealerPricingWrite,
  Permission.ProductSalesWrite,
  Permission.SuppliersManage,
  Permission.PurchasesWrite,
];

/** dealer_staff: service and catalog work, none of the dealer sales grants. */
const DEALER_STAFF_GRANTS = [
  Permission.CatalogRead,
  Permission.StockRead,
  Permission.AppointmentsRead,
];

const MODULES = ["accounting", "dealer_accounting"];

function visibleIds(
  granted: string[],
  org: { type: string; role: string; features: string[] },
) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: org as never,
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: dealer sales (TEC-348)", () => {
  it("links the four dealer sales pages behind dealer_accounting", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "dealer-sales");
    expect(group?.feature).toBe("dealer_accounting");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.dealerSales.quickSale("acme"),
      routes.tenant.dealerSales.prices("acme"),
      routes.tenant.dealerSales.suppliers("acme"),
      routes.tenant.dealerSales.purchases("acme"),
    ]);
  });

  it("shows for a dealer owner with the module on", () => {
    const ids = visibleIds(DEALER_SALES_GRANTS, {
      type: "dealer",
      role: "owner",
      features: MODULES,
    });
    for (const id of DEALER_SALES_ITEMS) expect(ids).toContain(id);
  });

  it("is hidden from the dealer_staff role", () => {
    const ids = visibleIds(DEALER_STAFF_GRANTS, {
      type: "dealer",
      role: "staff",
      features: MODULES,
    });
    for (const id of DEALER_SALES_ITEMS) expect(ids).not.toContain(id);
  });

  it("is hidden while the dealer_accounting module is off", () => {
    const ids = visibleIds(DEALER_SALES_GRANTS, {
      type: "dealer",
      role: "owner",
      features: ["accounting"],
    });
    for (const id of DEALER_SALES_ITEMS) expect(ids).not.toContain(id);
  });

  it("is hidden outside a dealer", () => {
    const ids = visibleIds(DEALER_SALES_GRANTS, {
      type: "distributor",
      role: "owner",
      features: MODULES,
    });
    for (const id of DEALER_SALES_ITEMS) expect(ids).not.toContain(id);
  });

  it("shows only the pages the permissions allow", () => {
    const ids = visibleIds([Permission.ProductSalesWrite], {
      type: "dealer",
      role: "accounting",
      features: MODULES,
    });
    expect(ids).toContain("dealer-sales-quick-sale");
    expect(ids).not.toContain("dealer-sales-prices");
    expect(ids).not.toContain("dealer-sales-purchases");
  });
});
