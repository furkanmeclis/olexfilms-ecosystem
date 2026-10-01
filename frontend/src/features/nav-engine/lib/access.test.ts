import { describe, expect, it } from "vitest";

import { isNavEntryVisible, visibleNavItems } from "./access";

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

describe("isNavEntryVisible module flags", () => {
  const org = { type: "dealer", role: "owner" };

  it("hides a module entry while the module list is unknown", () => {
    expect(
      isNavEntryVisible({ feature: "ai_assistant" }, { ...allow, org }),
    ).toBe(false);
    expect(
      isNavEntryVisible(
        { feature: "ai_assistant" },
        { ...allow, org: { ...org, features: null } },
      ),
    ).toBe(false);
  });

  it("hides a module entry when the module is off", () => {
    expect(
      isNavEntryVisible(
        { feature: "ai_assistant" },
        { ...allow, org: { ...org, features: ["leads"] } },
      ),
    ).toBe(false);
  });

  it("shows a module entry when the module is on", () => {
    expect(
      isNavEntryVisible(
        { feature: "ai_assistant" },
        { ...allow, org: { ...org, features: ["leads", "ai_assistant"] } },
      ),
    ).toBe(true);
  });

  it("filters items of a group by module", () => {
    const group = {
      id: "g",
      labelKey: "g",
      items: [
        {
          id: "a",
          titleKey: "a",
          href: "/a",
          icon: () => null,
          feature: "ai_assistant",
        },
        { id: "b", titleKey: "b", href: "/b", icon: () => null },
      ],
    };
    const items = visibleNavItems(group as never, {
      ...allow,
      org: { ...org, features: ["leads"] },
    });
    expect(items.map((i) => i.id)).toEqual(["b"]);
  });
});
