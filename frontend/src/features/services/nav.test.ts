import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[]) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type: "dealer", role: "owner", features: ["services"] },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

const wizardGrants = [
  Permission.ServicesWrite,
  Permission.CustomersRead,
  Permission.VehiclesRead,
];

describe("tenant nav: service wizard (TEC-181)", () => {
  it("links to the new service wizard", () => {
    const item = tenantNav("acme")
      .groups.flatMap((g) => g.items)
      .find((i) => i.id === "services-new");
    expect(item?.href).toBe(routes.tenant.services.create("acme"));
    expect(item?.feature).toBe("services");
  });

  it("is hidden without services.write", () => {
    expect(visibleIds([])).not.toContain("services-new");
    expect(
      visibleIds([
        Permission.ServicesRead,
        Permission.CustomersRead,
        Permission.VehiclesRead,
      ]),
    ).not.toContain("services-new");
  });

  it("shows with services.write and the customer/vehicle reads", () => {
    expect(visibleIds(wizardGrants)).toContain("services-new");
  });
});
