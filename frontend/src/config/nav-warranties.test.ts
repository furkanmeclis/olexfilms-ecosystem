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
 * TEC-866: the warranty list under "Services" is closed to dealers (owner,
 * admin and staff alike): no menu entry, no command palette page and the
 * route guard denies the list URL. Center and distributor keep it. The
 * detail pages stay open, since dealers reach them from the service detail.
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

  const catalogs = shellNavCatalogs(shellNav("tenant", SLUG));
  const route = (path: string, a: NavAccess) =>
    navRouteAccess(path, catalogs, a, `/t/${SLUG}`);
  const DETAIL = `/t/${SLUG}/warranties/0b6f1c1e-0000-4000-8000-000000000001`;

  it.each(["owner", "admin", "staff"])(
    "closes the list page to dealer %s but keeps the detail open",
    (role) => {
      const dealer = access("dealer", role);
      expect(route(`/t/${SLUG}/warranties`, dealer)).toBe("denied");
      expect(route(`/t/${SLUG}/warranties?status=active`, dealer)).toBe(
        "denied",
      );
      expect(route(DETAIL, dealer)).toBe("allowed");
    },
  );

  it.each(["center", "distributor"])("keeps the list open for %s", (type) => {
    expect(route(`/t/${SLUG}/warranties`, access(type, "staff"))).toBe(
      "allowed",
    );
    expect(route(DETAIL, access(type, "staff"))).toBe("allowed");
  });
});
