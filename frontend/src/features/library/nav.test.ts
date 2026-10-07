import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], features: string[], orgType: string) {
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

describe("tenant nav: documents (TEC-333)", () => {
  it("links the library page", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "library");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.library.list("acme"),
    ]);
  });

  it("shows Documents to every organization type with library.read", () => {
    for (const orgType of ["center", "distributor", "dealer"]) {
      expect(
        visibleIds([Permission.LibraryRead], ["announcements"], orgType),
      ).toContain("library-list");
    }
  });

  it("is hidden without library.read", () => {
    expect(
      visibleIds([Permission.AnnouncementsRead], ["announcements"], "dealer"),
    ).not.toContain("library-list");
  });
});
