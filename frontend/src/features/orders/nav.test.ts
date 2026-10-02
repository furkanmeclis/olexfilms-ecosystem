import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], type = "dealer") {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type, role: "owner", features: ["orders", "catalog"] },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

const buyer = [
  Permission.OrdersRead,
  Permission.OrdersWrite,
  Permission.CatalogRead,
];

describe("tenant nav: orders (TEC-170)", () => {
  it("links to the order list and the new order form", () => {
    const items = tenantNav("acme").groups.flatMap((g) => g.items);
    expect(items.find((i) => i.id === "orders-list")?.href).toBe(
      routes.tenant.orders.list("acme"),
    );
    expect(items.find((i) => i.id === "orders-new")?.href).toBe(
      routes.tenant.orders.create("acme"),
    );
  });

  it("is hidden without orders.read", () => {
    expect(visibleIds([])).not.toContain("orders-list");
  });

  it("shows the list with orders.read and the form to a buyer", () => {
    expect(visibleIds([Permission.OrdersRead])).toContain("orders-list");
    expect(visibleIds([Permission.OrdersRead])).not.toContain("orders-new");
    expect(visibleIds(buyer)).toContain("orders-new");
  });

  it("the center has no new order entry (K6)", () => {
    expect(visibleIds(buyer, "center")).toContain("orders-list");
    expect(visibleIds(buyer, "center")).not.toContain("orders-new");
  });
});
