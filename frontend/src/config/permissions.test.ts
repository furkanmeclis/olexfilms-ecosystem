import { describe, expect, it } from "vitest";

import {
  ALL_PERMISSIONS,
  broadestScope,
  scopeCovers,
} from "@/config/permissions";
import { pickGrants } from "@/features/roles/services/roles.service";

describe("permission scopes", () => {
  it("nests internal scopes and keeps customer apart", () => {
    expect(scopeCovers("subtree", "managed")).toBe(true);
    expect(scopeCovers("managed", "subtree")).toBe(false);
    expect(scopeCovers("brand", "subtree")).toBe(true);
    expect(scopeCovers("all", "customer")).toBe(false);
    expect(scopeCovers("customer", "customer")).toBe(true);
  });

  it("picks the broadest allowed scope", () => {
    expect(broadestScope(["managed", "subtree", "own"])).toBe("subtree");
    expect(broadestScope(["customer", "own"])).toBe("own");
    expect(broadestScope([])).toBeUndefined();
  });

  it("has no phantom tenant.* business permissions", () => {
    const tenant = ALL_PERMISSIONS.filter((slug) => slug.startsWith("tenant."));
    expect(tenant.sort()).toEqual([
      "tenant.exports.read",
      "tenant.imports.read",
      "tenant.settings.read",
      "tenant.settings.write",
    ]);
  });

  it("sends scopes of selected permissions only", () => {
    expect(
      pickGrants(["services.read"], {
        "services.read": "managed",
        "pricing.sale.read": "managed",
      }),
    ).toEqual({ "services.read": "managed" });
  });
});
