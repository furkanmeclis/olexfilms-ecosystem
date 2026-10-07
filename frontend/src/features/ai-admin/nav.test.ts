import { describe, expect, it } from "vitest";

import { platformNav, tenantNav } from "@/config/nav";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import type { NavCatalog } from "@/features/nav-engine/types";

const hrefs = (
  catalog: NavCatalog,
  perms: string[],
  type: "center" | "distributor" | "dealer" = "dealer",
) =>
  catalog.groups.flatMap((group) =>
    visibleNavItems(group, {
      can: (p) => (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
      canAny: (ps) => ps.some((x) => perms.includes(x)),
      org: { type, role: "owner", features: [] },
    }).map((item) => item.href),
  );

describe("AI admin menu (TEC-391)", () => {
  it("shows the AI usage page only with ai.usage.read", () => {
    for (const type of ["center", "distributor", "dealer"] as const) {
      expect(hrefs(tenantNav("acme"), ["ai.usage.read"], type)).toContain(
        "/t/acme/ai-usage",
      );
      expect(hrefs(tenantNav("acme"), ["ai.use"], type)).not.toContain(
        "/t/acme/ai-usage",
      );
      expect(hrefs(tenantNav("acme"), [], type)).not.toContain(
        "/t/acme/ai-usage",
      );
    }
  });

  it("shows the platform AI screen only with ai.settings.manage", () => {
    expect(hrefs(platformNav, ["ai.settings.manage"])).toContain(
      "/platform/ai",
    );
    expect(hrefs(platformNav, ["ai.usage.read"])).not.toContain("/platform/ai");
  });
});
