import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], features: string[]) {
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

describe("tenant nav: fleets (TEC-477)", () => {
  it("hides the fleets menu while the fleet module is off", () => {
    expect(visibleIds([Permission.FleetsRead], ["services"])).not.toContain(
      "fleets-list",
    );
  });

  it("shows the fleets menu with the fleet module and fleets.read", () => {
    expect(visibleIds([Permission.FleetsRead], ["fleet"])).toContain(
      "fleets-list",
    );
    expect(visibleIds([], ["fleet"])).not.toContain("fleets-list");
  });

  it("links to the fleet list", () => {
    const item = tenantNav("acme")
      .groups.flatMap((g) => g.items)
      .find((i) => i.id === "fleets-list");
    expect(item?.href).toBe(routes.tenant.fleets.list("acme"));
  });
});
