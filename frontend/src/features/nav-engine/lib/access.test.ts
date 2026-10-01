import { describe, expect, it } from "vitest";

import { isNavEntryVisible } from "./access";

const allow = { can: () => true, canAny: () => true };

describe("isNavEntryVisible org filters", () => {
  it("hides org-typed entries without an active organization", () => {
    expect(isNavEntryVisible({ orgTypes: ["distributor"] }, allow)).toBe(false);
  });

  it("matches organization type", () => {
    const entry = { orgTypes: ["center", "distributor"] as const };
    expect(
      isNavEntryVisible(
        { orgTypes: [...entry.orgTypes] },
        { ...allow, org: { type: "distributor", role: "staff" } },
      ),
    ).toBe(true);
    expect(
      isNavEntryVisible(
        { orgTypes: [...entry.orgTypes] },
        { ...allow, org: { type: "dealer", role: "owner" } },
      ),
    ).toBe(false);
  });

  it("matches member role", () => {
    expect(
      isNavEntryVisible(
        { orgRoles: ["owner"] },
        { ...allow, org: { type: "dealer", role: "staff" } },
      ),
    ).toBe(false);
    expect(
      isNavEntryVisible(
        { orgRoles: ["owner"] },
        { ...allow, org: { type: "dealer", role: "owner" } },
      ),
    ).toBe(true);
  });

  it("keeps entries without org filters visible", () => {
    expect(isNavEntryVisible({}, allow)).toBe(true);
  });
});
