import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  isNavEntryVisible,
  type NavAccess,
} from "@/features/nav-engine/lib/access";
import { navRouteAccess } from "@/features/nav-engine/lib/route-access";

/**
 * TEC-871: the campaign approval queue (GET /v1/campaigns/approvals) needs
 * campaigns.approve. A dealer only has campaigns.read/write, so neither the
 * menu (and with it the command palette) nor the route guard may offer it.
 */
const SLUG = "acme";

function access(perms: string[], type: string): NavAccess {
  return {
    can: (p) => (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
    canAny: (ps) => ps.some((x) => perms.includes(x)),
    org: { type, role: "owner", features: ["campaigns"] },
  };
}

function visibleCampaignItems(a: NavAccess): string[] {
  const group = tenantNav(SLUG).groups.find((g) => g.id === "campaigns");
  if (!group || !isNavEntryVisible(group, a)) return [];
  return group.items
    .filter((item) => isNavEntryVisible(item, a))
    .map((item) => item.id);
}

const dealer = access(
  [Permission.CampaignsRead, Permission.CampaignsWrite],
  "dealer",
);
const center = access(
  [
    Permission.CampaignsRead,
    Permission.CampaignsWrite,
    Permission.CampaignsApprove,
  ],
  "center",
);

describe("campaign approval queue access (TEC-871)", () => {
  it("hides the approval queue from a dealer", () => {
    expect(visibleCampaignItems(dealer)).toEqual([
      "campaigns-list",
      "campaigns-new",
    ]);
  });

  it("shows it to an approver with campaigns.approve", () => {
    expect(visibleCampaignItems(center)).toEqual([
      "campaigns-list",
      "campaigns-new",
      "campaigns-approvals",
    ]);
  });

  it("denies the route for a dealer and allows it for an approver", () => {
    const catalogs = [tenantNav(SLUG)];
    const path = routes.tenant.campaigns.approvals(SLUG);
    const home = routes.tenant.home(SLUG);
    expect(navRouteAccess(path, catalogs, dealer, home)).toBe("denied");
    expect(navRouteAccess(path, catalogs, center, home)).toBe("allowed");
    expect(
      navRouteAccess(routes.tenant.campaigns.list(SLUG), catalogs, dealer, home),
    ).toBe("allowed");
  });
});
