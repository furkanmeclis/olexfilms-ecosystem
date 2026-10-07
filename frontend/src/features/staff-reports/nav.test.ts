import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { resolveStaffAccess } from "@/features/staff-reports/hooks/use-staff-access";

function visibleIds(
  granted: string[],
  type: string,
  features: string[] = ["accounting", "dealer_accounting"],
) {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type, role: "owner", features },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: staff and reports (TEC-349)", () => {
  it("links the staff and report pages", () => {
    const group = tenantNav("acme").groups.find(
      (g) => g.id === "staff-reports",
    );
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.staff.list("acme"),
      routes.tenant.accountingReports("acme"),
    ]);
  });

  it("a dealer owner sees staff and reports", () => {
    const ids = visibleIds(
      [Permission.StaffManage, Permission.AccountingRead],
      "dealer",
    );
    expect(ids).toContain("staff-reports-staff");
    expect(ids).toContain("staff-reports-reports");
  });

  it("dealer_staff (no staff.manage, no accounting.read) sees neither", () => {
    const ids = visibleIds([Permission.CatalogRead], "dealer");
    expect(ids).not.toContain("staff-reports-staff");
    expect(ids).not.toContain("staff-reports-reports");
  });

  it("staff cards hide while dealer_accounting is off", () => {
    const ids = visibleIds(
      [Permission.StaffManage, Permission.AccountingRead],
      "dealer",
      ["accounting"],
    );
    expect(ids).not.toContain("staff-reports-staff");
    expect(ids).toContain("staff-reports-reports");
  });
});

describe("resolveStaffAccess", () => {
  const all = (granted: string[]) => (p: string) => granted.includes(p);

  it("needs the dealer_accounting module in a dealer", () => {
    const can = all([Permission.StaffManage, Permission.StaffPaymentsWrite]);
    expect(
      resolveStaffAccess({ can, orgType: "dealer", features: ["accounting"] }),
    ).toMatchObject({ canManage: false, canPay: false });
    expect(
      resolveStaffAccess({
        can,
        orgType: "dealer",
        features: ["accounting", "dealer_accounting"],
      }),
    ).toMatchObject({ canManage: true, canPay: true });
  });

  it("payments need staff_payments.write", () => {
    expect(
      resolveStaffAccess({
        can: all([Permission.StaffManage]),
        orgType: "dealer",
        features: ["accounting", "dealer_accounting"],
      }),
    ).toMatchObject({ canManage: true, canPay: false });
  });
});
