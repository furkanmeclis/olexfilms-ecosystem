import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

const ACCOUNTING_ITEMS = [
  "accounting-accounts",
  "accounting-cari",
  "accounting-entries",
];

function visibleIds(granted: string[], features: string[] = ["accounting"]) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type: "distributor", role: "owner", features },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: accounting (TEC-176)", () => {
  it("links the accounting pages", () => {
    const items = tenantNav("acme").groups.find((g) => g.id === "accounting");
    expect(items?.feature).toBe("accounting");
    expect(items?.items.map((i) => i.href)).toEqual([
      routes.tenant.accounting.accounts("acme"),
      routes.tenant.accounting.cari("acme"),
      routes.tenant.accounting.entries("acme"),
    ]);
  });

  it("is hidden from a user without accounting.read", () => {
    const ids = visibleIds([Permission.CatalogRead]);
    for (const id of ACCOUNTING_ITEMS) expect(ids).not.toContain(id);
  });

  it("is hidden while the accounting module is off", () => {
    const ids = visibleIds([Permission.AccountingRead], []);
    for (const id of ACCOUNTING_ITEMS) expect(ids).not.toContain(id);
  });

  it("shows with accounting.read and the module on", () => {
    const ids = visibleIds([Permission.AccountingRead]);
    for (const id of ACCOUNTING_ITEMS) expect(ids).toContain(id);
  });
});
