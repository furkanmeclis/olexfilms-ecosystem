import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], features = ["services"]) {
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

describe("tenant nav: certificates (TEC-482)", () => {
  it("shows certificate screens only while the certificates module is enabled", () => {
    expect(
      visibleIds([Permission.CertificatesRead], ["services"]),
    ).not.toContain("certificates-list");
    expect(
      visibleIds([Permission.CertificatesRead], ["services", "certificates"]),
    ).toContain("certificates-list");
  });
});

describe("tenant nav: service list (TEC-183)", () => {
  it("links to the service list with services.read alone", () => {
    const item = tenantNav("acme")
      .groups.flatMap((g) => g.items)
      .find((i) => i.id === "services-list");
    expect(item?.href).toBe(routes.tenant.services.list("acme"));
    expect(visibleIds([Permission.ServicesRead])).toContain("services-list");
    expect(visibleIds([Permission.ServicesRead])).not.toContain("services-new");
  });

  it("is hidden without services.read", () => {
    expect(visibleIds([])).not.toContain("services-list");
    expect(visibleIds(wizardGrants)).not.toContain("services-list");
  });
});
