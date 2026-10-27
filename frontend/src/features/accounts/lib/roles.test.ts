import { describe, expect, it } from "vitest";

import { isWarehouseRole, staffRoles } from "./roles";

describe("staff roles", () => {
  it("lists the staff roles of the organization type, never an owner role", () => {
    expect(staffRoles("center")[0]).toBe("center_warehouse");
    expect(staffRoles("dealer")).toContain("dealer_staff");
    for (const type of ["center", "distributor", "dealer"]) {
      expect(staffRoles(type).some((r) => r.endsWith("_owner"))).toBe(false);
    }
    expect(staffRoles(undefined)).toEqual([]);
  });
  it("knows the warehouse roles", () => {
    expect(isWarehouseRole("center_warehouse")).toBe(true);
    expect(isWarehouseRole("center_staff")).toBe(false);
  });
});
