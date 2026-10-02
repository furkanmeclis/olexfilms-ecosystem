import { describe, expect, it } from "vitest";

import { Permission } from "@/config/permissions";
import { resolveAccountingAccess } from "@/features/accounting/lib/access";

const can = (granted: string[]) => (p: string) => granted.includes(p);
const rw = [Permission.AccountingRead, Permission.AccountingWrite];

describe("resolveAccountingAccess", () => {
  it("hides everything without accounting.read", () => {
    expect(
      resolveAccountingAccess({ can: can([]), orgType: "center" }),
    ).toEqual({ canRead: false, canWrite: false });
  });

  it("center and distributor write with accounting.write", () => {
    for (const orgType of ["center", "distributor"]) {
      expect(resolveAccountingAccess({ can: can(rw), orgType })).toEqual({
        canRead: true,
        canWrite: true,
      });
    }
    expect(
      resolveAccountingAccess({
        can: can([Permission.AccountingRead]),
        orgType: "center",
      }).canWrite,
    ).toBe(false);
  });

  it("a dealer is read only unless dealer_accounting is on", () => {
    expect(
      resolveAccountingAccess({ can: can(rw), orgType: "dealer", features: [] })
        .canWrite,
    ).toBe(false);
    expect(
      resolveAccountingAccess({
        can: can(rw),
        orgType: "dealer",
        features: ["accounting", "dealer_accounting"],
      }).canWrite,
    ).toBe(true);
  });
});
