import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], features = ["customers"]) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type: "dealer", role: "owner", features },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: customers (TEC-163)", () => {
  it("links to the customer list and the new customer form", () => {
    const items = tenantNav("acme").groups.flatMap((g) => g.items);
    expect(items.find((i) => i.id === "customers-list")?.href).toBe(
      routes.tenant.customers.list("acme"),
    );
    expect(items.find((i) => i.id === "customers-new")?.href).toBe(
      routes.tenant.customers.create("acme"),
    );
  });

  it("is hidden without customers.read", () => {
    expect(visibleIds([])).not.toContain("customers-list");
  });

  it("is hidden while the customers module is off", () => {
    expect(visibleIds([Permission.CustomersRead], [])).not.toContain(
      "customers-list",
    );
  });

  it("shows the form only with customers.write", () => {
    expect(visibleIds([Permission.CustomersRead])).toContain("customers-list");
    expect(visibleIds([Permission.CustomersRead])).not.toContain(
      "customers-new",
    );
    expect(
      visibleIds([Permission.CustomersRead, Permission.CustomersWrite]),
    ).toContain("customers-new");
  });
});

describe("tenant nav: vehicles (TEC-372)", () => {
  it("links to the vehicle list", () => {
    const items = tenantNav("acme").groups.flatMap((g) => g.items);
    expect(items.find((i) => i.id === "vehicles-list")?.href).toBe(
      routes.tenant.vehicles.list("acme"),
    );
  });

  it("needs vehicles.read and the customers module", () => {
    expect(visibleIds([Permission.CustomersRead])).not.toContain(
      "vehicles-list",
    );
    expect(
      visibleIds([Permission.CustomersRead, Permission.VehiclesRead], []),
    ).not.toContain("vehicles-list");
    expect(
      visibleIds([Permission.CustomersRead, Permission.VehiclesRead]),
    ).toContain("vehicles-list");
  });
});
