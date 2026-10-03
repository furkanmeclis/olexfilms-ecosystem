import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(
  granted: string[],
  orgType = "center",
  features: string[] = ["warehouse"],
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

describe("tenant nav: warehouse (TEC-231)", () => {
  it("links the slice-1 screens", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "warehouse");
    expect(group?.feature).toBe("warehouse");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.warehouse.locations("acme"),
      routes.tenant.warehouse.scan("acme"),
      routes.tenant.warehouse.entries("acme"),
      routes.tenant.warehouse.barcodes("acme"),
    ]);
  });

  it("shows the warehouse to the center and the distributor with warehouse.read", () => {
    const read = [Permission.WarehouseRead];
    for (const type of ["center", "distributor"]) {
      const ids = visibleIds(read, type);
      expect(ids).toContain("warehouse-locations");
      expect(ids).toContain("warehouse-scan");
      expect(ids).toContain("warehouse-entries");
    }
  });

  it("barcodes: center only, with stock.read", () => {
    const both = [Permission.WarehouseRead, Permission.StockRead];
    expect(visibleIds(both, "center")).toContain("warehouse-barcodes");
    expect(visibleIds(both, "distributor")).not.toContain("warehouse-barcodes");
    expect(visibleIds([Permission.WarehouseRead], "center")).not.toContain(
      "warehouse-barcodes",
    );
  });

  it("is hidden for a dealer, without warehouse.read or with the module off", () => {
    const read = [Permission.WarehouseRead, Permission.StockRead];
    expect(visibleIds(read, "dealer")).not.toContain("warehouse-locations");
    expect(visibleIds([Permission.StockRead], "center")).not.toContain(
      "warehouse-locations",
    );
    expect(visibleIds(read, "center", [])).not.toContain("warehouse-locations");
  });
});
