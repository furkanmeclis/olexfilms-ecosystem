import { describe, expect, it } from "vitest";

import { platformNav, tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visiblePlatformIds(granted: string[]) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
  };
  return platformNav.groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

function visibleTenantIds(granted: string[], features: string[]) {
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

describe("service catalog nav (TEC-310)", () => {
  it("links platform menu to service catalog management", () => {
    const item = platformNav.groups
      .flatMap((g) => g.items)
      .find((i) => i.id === "service-catalog");
    expect(item?.href).toBe(routes.platform.serviceCatalog.root);
    expect(item?.permission).toBe(Permission.ServiceCatalogManage);
    expect(visiblePlatformIds([])).not.toContain("service-catalog");
    expect(visiblePlatformIds([Permission.ServiceCatalogManage])).toContain(
      "service-catalog",
    );
  });

  it("links tenant menu to the read-only catalog", () => {
    const item = tenantNav("acme")
      .groups.flatMap((g) => g.items)
      .find((i) => i.id === "service-catalog");
    expect(item?.href).toBe(routes.tenant.catalog.services("acme"));
    expect(item?.permission).toBe(Permission.ServiceCatalogRead);
    expect(visibleTenantIds([Permission.ServiceCatalogRead], [])).not.toContain(
      "service-catalog",
    );
    expect(
      visibleTenantIds([Permission.ServiceCatalogRead], ["service_catalog"]),
    ).toContain("service-catalog");
  });
});
