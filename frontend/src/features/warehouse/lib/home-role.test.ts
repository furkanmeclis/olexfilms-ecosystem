import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { isWarehouseHomeRole } from "@/features/warehouse/lib/home-role";

// Grants of the system roles that matter here (rbac/catalog.go).
const CENTER_WAREHOUSE = [
  Permission.WarehouseRead,
  Permission.StockRead,
  Permission.OrdersRead,
  Permission.OrdersShip,
  Permission.OrganizationsRead,
  Permission.CatalogRead,
];
const DISTRIBUTOR_WAREHOUSE = [
  Permission.WarehouseRead,
  Permission.StockRead,
  Permission.OrdersRead,
  Permission.OrdersShip,
  Permission.CatalogRead,
];
const CENTER_OWNER = [
  ...CENTER_WAREHOUSE,
  Permission.ServicesWrite,
  Permission.AccountingRead,
  Permission.CustomersRead,
];

const canOf = (list: string[]) => (p: string) => list.includes(p);

describe("isWarehouseHomeRole (TEC-1066)", () => {
  it("sends the center and distributor warehouse staff to the warehouse home", () => {
    expect(isWarehouseHomeRole(canOf(CENTER_WAREHOUSE), "center")).toBe(true);
    expect(
      isWarehouseHomeRole(canOf(DISTRIBUTOR_WAREHOUSE), "distributor"),
    ).toBe(true);
  });

  it("keeps the general home for combined accounts and dealers", () => {
    expect(isWarehouseHomeRole(canOf(CENTER_OWNER), "center")).toBe(false);
    expect(
      isWarehouseHomeRole(
        canOf([...CENTER_WAREHOUSE, Permission.AccountingRead]),
        "center",
      ),
    ).toBe(false);
    expect(isWarehouseHomeRole(canOf(CENTER_WAREHOUSE), "dealer")).toBe(false);
    expect(isWarehouseHomeRole(canOf(CENTER_WAREHOUSE), undefined)).toBe(false);
  });
});

describe("tenant nav for the warehouse role (TEC-1066)", () => {
  const visible = (perms: string[], type: string) => {
    const access = {
      can: (p: string | string[]) =>
        (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
      canAny: (list: string[]) => list.some((x) => perms.includes(x)),
      org: {
        type,
        role: "staff",
        features: [
          "customers",
          "services",
          "orders",
          "stock",
          "warehouse",
          "catalog",
          "fleet",
          "appointments",
          "measurements",
          "warranty_claims",
        ],
      },
    };
    return tenantNav("acme").groups.flatMap((g) =>
      visibleNavItems(g, access).map((i) => i.id),
    );
  };

  it("hides customers, vehicles, services and warranties (menu and route)", () => {
    const ids = visible(CENTER_WAREHOUSE, "center");
    for (const id of [
      "customers-list",
      "customers-new",
      "vehicles-list",
      "services-list",
      "services-new",
      "warranties-list",
      "fleets-list",
    ]) {
      expect(ids).not.toContain(id);
    }
    expect(ids).toEqual(expect.arrayContaining(["orders-list", "home"]));
  });

  it("shows the read-only dealer directory to roles that read organizations", () => {
    expect(visible(CENTER_WAREHOUSE, "center")).toContain("dealers-list");
    // distributor warehouse staff hold no organizations.read (no migration
    // in TEC-1066), so the directory stays hidden for them.
    expect(visible(DISTRIBUTOR_WAREHOUSE, "distributor")).not.toContain(
      "dealers-list",
    );
    expect(visible(CENTER_OWNER, "dealer")).not.toContain("dealers-list");
  });
});
