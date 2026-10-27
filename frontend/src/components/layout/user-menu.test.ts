import { describe, expect, it } from "vitest";

import { resolveProfileHref } from "@/components/layout/user-menu";
import type { AuthUser } from "@/lib/auth/types";

function user(over: Partial<AuthUser>): AuthUser {
  return {
    uuid: "u",
    email: "u@example.com",
    name: "U",
    surname: "S",
    fullName: "U S",
    status: "active",
    locale: "tr",
    ownLocale: null,
    timeZone: "Europe/Istanbul",
    ownTimeZone: null,
    isSuperAdmin: false,
    emailVerified: true,
    permissions: [],
    grants: {},
    roles: [],
    organizationRoles: [],
    organizations: [],
    ...over,
  };
}

const centerOrg = {
  uuid: "o",
  slug: "olex",
  name: "Olex",
  role: "staff",
  status: "active",
  type: "center" as const,
};

describe("resolveProfileHref (TEC-1033)", () => {
  it("sends center staff with platform.* grants to the tenant profile", () => {
    const staff = user({
      roles: ["center_staff"],
      permissions: ["platform.showcase.review"],
      organizations: [centerOrg],
    });
    expect(resolveProfileHref("/platform", staff)).toBe("/t/olex/profile");
    expect(resolveProfileHref("/t/olex/settings", staff)).toBe(
      "/t/olex/profile",
    );
  });

  it("keeps the platform profile for the platform admin", () => {
    const admin = user({ isSuperAdmin: true, roles: ["super_admin"] });
    expect(resolveProfileHref("/platform", admin)).toBe("/platform/profile");
    expect(resolveProfileHref("/t/olex", admin)).toBe("/platform/profile");
  });
});
