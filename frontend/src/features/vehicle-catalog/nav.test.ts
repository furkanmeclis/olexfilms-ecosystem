import { describe, expect, it } from "vitest";

import { platformNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[]) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
  };
  return platformNav.groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("platform nav: vehicle catalog (TEC-150)", () => {
  it("links to the vehicle catalog page", () => {
    const item = platformNav.groups
      .flatMap((g) => g.items)
      .find((i) => i.id === "vehicle-catalog");
    expect(item?.href).toBe(routes.platform.vehicleCatalog.root);
    expect(item?.permission).toBe(Permission.VehicleCatalogWrite);
  });

  it("is hidden without vehicle_catalog.write", () => {
    expect(visibleIds([])).not.toContain("vehicle-catalog");
    expect(visibleIds([Permission.VehicleCatalogRead])).not.toContain(
      "vehicle-catalog",
    );
  });

  it("shows with vehicle_catalog.write", () => {
    expect(visibleIds([Permission.VehicleCatalogWrite])).toContain(
      "vehicle-catalog",
    );
  });
});
