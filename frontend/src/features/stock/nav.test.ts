import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(
  granted: string[],
  orgType = "dealer",
  features: string[] = ["stock"],
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

describe("tenant nav: my stock (TEC-224)", () => {
  it("links the stock page", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "stock");
    expect(group?.feature).toBe("stock");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.stock.root("acme"),
    ]);
  });

  it("shows the page to a dealer and a distributor with stock.read", () => {
    expect(visibleIds([Permission.StockRead])).toContain("stock-mine");
    expect(visibleIds([Permission.StockRead], "distributor")).toContain(
      "stock-mine",
    );
  });

  it("is hidden without stock.read, with the module off, or for the center", () => {
    expect(visibleIds([Permission.CatalogRead])).not.toContain("stock-mine");
    expect(visibleIds([Permission.StockRead], "dealer", [])).not.toContain(
      "stock-mine",
    );
    expect(visibleIds([Permission.StockRead], "center")).not.toContain(
      "stock-mine",
    );
  });
});
