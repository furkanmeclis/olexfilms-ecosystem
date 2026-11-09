import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { queueTabOf } from "@/features/accounting/components/accounting-queue-page";
import { isAccountingHomeRole } from "@/features/accounting/lib/home-role";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

// Grants of the system roles that matter here (rbac/catalog.go).
const CENTER_ACCOUNTING = [
  Permission.AccountingRead,
  Permission.AccountingWrite,
  Permission.OrdersRead,
  Permission.OrganizationsRead,
  Permission.CatalogRead,
  Permission.EinvoiceManage,
];
const CENTER_OWNER = [
  ...CENTER_ACCOUNTING,
  Permission.ServicesWrite,
  Permission.WarehouseRead,
  Permission.CustomersRead,
];
const canOf = (list: string[]) => (p: string) => list.includes(p);

describe("isAccountingHomeRole (TEC-1070)", () => {
  it("sends center and distributor accounting to the accounting home", () => {
    expect(isAccountingHomeRole(canOf(CENTER_ACCOUNTING), "center")).toBe(true);
    expect(
      isAccountingHomeRole(
        canOf([Permission.AccountingRead, Permission.OrdersRead]),
        "distributor",
      ),
    ).toBe(true);
  });

  it("keeps the general home for combined accounts and dealers", () => {
    expect(isAccountingHomeRole(canOf(CENTER_OWNER), "center")).toBe(false);
    expect(
      isAccountingHomeRole(
        canOf([...CENTER_ACCOUNTING, Permission.WarehouseRead]),
        "center",
      ),
    ).toBe(false);
    expect(isAccountingHomeRole(canOf(CENTER_ACCOUNTING), "dealer")).toBe(
      false,
    );
  });
});

describe("accounting nav (TEC-1070)", () => {
  const ids = (perms: string[], type: string) => {
    const access = {
      can: (p: string | string[]) =>
        (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
      canAny: (list: string[]) => list.some((x) => perms.includes(x)),
      org: {
        type,
        role: "staff",
        features: ["accounting", "orders", "customers", "services", "stock"],
      },
    };
    return tenantNav("acme").groups.flatMap((g) =>
      visibleNavItems(g, access).map((i) => i.id),
    );
  };

  it("puts the queue and returns first in the accounting group", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "accounting");
    expect(group?.items.slice(0, 2).map((i) => i.id)).toEqual([
      "accounting-queue",
      "accounting-returns",
    ]);
  });

  it("shows orders, the queue and the caris; no customers or services", () => {
    const visible = ids(CENTER_ACCOUNTING, "center");
    expect(visible).toEqual(
      expect.arrayContaining([
        "orders-list",
        "accounting-queue",
        "accounting-returns",
        "accounting-cari",
      ]),
    );
    for (const id of ["customers-list", "services-list", "warranties-list"]) {
      expect(visible).not.toContain(id);
    }
    expect(ids(CENTER_ACCOUNTING, "dealer")).not.toContain("accounting-queue");
  });
});

describe("queueTabOf", () => {
  it("falls back to the orders tab", () => {
    expect(queueTabOf("returns")).toBe("returns");
    expect(queueTabOf("nope")).toBe("orders");
    expect(queueTabOf(null)).toBe("orders");
  });
});
