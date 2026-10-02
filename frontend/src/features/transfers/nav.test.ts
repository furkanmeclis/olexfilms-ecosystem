import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(
  granted: string[],
  orgType = "dealer",
  features: string[] = ["dealer_transfers"],
) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type: orgType, role: "owner", features },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: stock transfers (TEC-197)", () => {
  it("links the transfer pages", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "transfers");
    expect(group?.feature).toBe("dealer_transfers");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.transfers.list("acme"),
      routes.tenant.transfers.create("acme"),
    ]);
  });

  it("shows list and new to a dealer with transfers.request", () => {
    const ids = visibleIds([Permission.TransfersRequest]);
    expect(ids).toContain("transfers-list");
    expect(ids).toContain("transfers-new");
  });

  it("shows only the list to an approver", () => {
    const ids = visibleIds([Permission.TransfersApprove], "distributor");
    expect(ids).toContain("transfers-list");
    expect(ids).not.toContain("transfers-new");
  });

  it("is hidden without a transfer permission or with the module off", () => {
    expect(visibleIds([Permission.CatalogRead])).not.toContain(
      "transfers-list",
    );
    expect(
      visibleIds([Permission.TransfersRequest], "dealer", []),
    ).not.toContain("transfers-list");
  });

  it("hides new from the center", () => {
    expect(visibleIds([Permission.TransfersRequest], "center")).not.toContain(
      "transfers-new",
    );
  });
});
