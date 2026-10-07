import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], features = ["appointments"]) {
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

describe("tenant nav: appointments (TEC-326)", () => {
  it("links the calendar", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "appointments");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.appointments.calendar("acme"),
    ]);
  });

  it("shows the calendar with appointments.read and the module", () => {
    expect(visibleIds([Permission.AppointmentsRead])).toContain(
      "appointments-calendar",
    );
  });

  it("has no menu without the permission or the module", () => {
    expect(visibleIds([Permission.ServicesRead])).not.toContain(
      "appointments-calendar",
    );
    expect(
      visibleIds([Permission.AppointmentsRead], ["services"]),
    ).not.toContain("appointments-calendar");
  });
});
