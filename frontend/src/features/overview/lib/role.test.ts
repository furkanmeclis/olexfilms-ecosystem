import { describe, expect, it } from "vitest";

import { Permission } from "@/config/permissions";
import { resolveOverviewRole } from "@/features/overview/lib/role";

const grants =
  (...list: string[]) =>
  (p: string) =>
    list.includes(p);

describe("resolveOverviewRole", () => {
  it("routes accounting-only center members to the accounting overview", () => {
    expect(
      resolveOverviewRole(grants(Permission.AccountingRead), "center"),
    ).toBe("accounting");
  });

  it("routes warehouse-only distributor members to the warehouse overview", () => {
    expect(
      resolveOverviewRole(grants(Permission.WarehouseRead), "distributor"),
    ).toBe("warehouse");
  });

  it("keeps combined accounts on the organization overview", () => {
    const can = grants(
      Permission.AccountingRead,
      Permission.WarehouseRead,
      Permission.ServicesWrite,
    );
    expect(resolveOverviewRole(can, "center")).toBe("center");
    expect(resolveOverviewRole(can, "distributor")).toBe("distributor");
  });

  it("uses the organization type for dealers", () => {
    expect(
      resolveOverviewRole(grants(Permission.AccountingRead), "dealer"),
    ).toBe("dealer");
    expect(resolveOverviewRole(grants(), undefined)).toBeNull();
  });
});
