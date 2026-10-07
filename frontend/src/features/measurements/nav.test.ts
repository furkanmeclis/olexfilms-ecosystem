import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], features = ["measurements"]) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type: "dealer", role: "owner", features },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: measurements (TEC-299)", () => {
  it("links the measurement pages", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "measurements");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.measurements.list("acme"),
      routes.tenant.measurements.devices("acme"),
    ]);
  });

  it("shows the list with measurements.read", () => {
    const ids = visibleIds([Permission.MeasurementsRead]);
    expect(ids).toContain("measurements-list");
    expect(ids).not.toContain("measurement-devices");
  });

  it("shows the devices with measurement_devices.manage", () => {
    const ids = visibleIds([Permission.MeasurementDevicesManage]);
    expect(ids).toContain("measurement-devices");
    expect(ids).not.toContain("measurements-list");
  });

  it("has no menu without the permissions or the module", () => {
    const ids = visibleIds([Permission.ServicesRead, Permission.CatalogRead]);
    expect(ids).not.toContain("measurements-list");
    expect(ids).not.toContain("measurement-devices");
    expect(
      visibleIds([Permission.MeasurementsRead], ["services"]),
    ).not.toContain("measurements-list");
  });
});
