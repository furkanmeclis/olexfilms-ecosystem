import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { shellNav, shellNavCatalogs } from "@/config/shell-nav";
import {
  type NavAccess,
  visibleNavItems,
} from "@/features/nav-engine/lib/access";
import { navRouteAccess } from "@/features/nav-engine/lib/route-access";
import { buildNavPageItems } from "@/features/search-engine/lib/nav-pages";

/**
 * TEC-866: the warranty list under "Services" is left out of the dealer
 * menu (owner and staff alike) and command palette; center and distributor
 * keep it. The page stays open, since dealers reach warranty details from
 * the service detail.
 */
const SLUG = "acme";

function access(type: string, role: string): NavAccess {
  return {
    can: () => true,
    canAny: () => true,
    org: { type, role, features: ["services", "warranty_claims"] },
  };
}

function servicesMenuIds(a: NavAccess) {
  const group = tenantNav(SLUG).groups.find((g) => g.id === "services");
  if (!group) throw new Error("services group missing");
  return visibleNavItems(group, a).map((item) => item.id);
}

describe("warranty list menu entry (TEC-866)", () => {
  it.each([
    ["dealer", "owner"],
    ["dealer", "admin"],
    ["dealer", "staff"],
  ])("is hidden for %s %s", (type, role) => {
    const ids = servicesMenuIds(access(type, role));
    expect(ids).not.toContain("warranties-list");
    expect(ids).toContain("services-list");
    expect(ids).toContain("warranty-claims-list");
  });

  it.each([
    ["center", "owner"],
    ["distributor", "staff"],
  ])("stays for %s %s", (type, role) => {
    expect(servicesMenuIds(access(type, role))).toContain("warranties-list");
  });

  it("is left out of the dealer command palette only", () => {
    const pages = (type: string) =>
      buildNavPageItems("tenant", access(type, "owner"), (k) => k, SLUG).map(
        (item) => item.id,
      );
    expect(pages("dealer")).not.toContain("page:warranties-list");
    expect(pages("center")).toContain("page:warranties-list");
  });

  it("keeps the warranty pages open for dealers", () => {
    const catalogs = shellNavCatalogs(shellNav("tenant", SLUG));
    const dealer = access("dealer", "staff");
    expect(
      navRouteAccess(`/t/${SLUG}/warranties`, catalogs, dealer, `/t/${SLUG}`),
    ).toBe("allowed");
    expect(
      navRouteAccess(
        `/t/${SLUG}/warranties/0b6f1c1e-0000-4000-8000-000000000001`,
        catalogs,
        dealer,
        `/t/${SLUG}`,
      ),
    ).toBe("allowed");
  });
});
