import { describe, expect, it } from "vitest";

import { Permission } from "@/config/permissions";
import { resolveAccountingAccess } from "@/features/accounting/lib/access";

const can = (granted: string[]) => (p: string) => granted.includes(p);
const rw = [Permission.AccountingRead, Permission.AccountingWrite];

describe("resolveAccountingAccess", () => {
  it("hides everything without accounting.read", () => {
    expect(
      resolveAccountingAccess({
        can: can([Permission.AccountingDispute, Permission.AccountingResolve]),
        orgType: "distributor",
        parentUuid: "center",
      }),
    ).toEqual({
      canRead: false,
      canWrite: false,
      canDispute: false,
      canResolve: false,
      readOnlyDealer: false,
    });
  });

  it("center and distributor write with accounting.write", () => {
    for (const orgType of ["center", "distributor"]) {
      const access = resolveAccountingAccess({ can: can(rw), orgType });
      expect(access.canRead).toBe(true);
      expect(access.canWrite).toBe(true);
      expect(access.readOnlyDealer).toBe(false);
    }
    expect(
      resolveAccountingAccess({
        can: can([Permission.AccountingRead]),
        orgType: "center",
      }).canWrite,
    ).toBe(false);
  });

  it("a dealer is read only unless dealer_accounting is on", () => {
    const off = resolveAccountingAccess({
      can: can(rw),
      orgType: "dealer",
      features: [],
    });
    expect(off.canWrite).toBe(false);
    expect(off.readOnlyDealer).toBe(true);
    const on = resolveAccountingAccess({
      can: can(rw),
      orgType: "dealer",
      features: ["accounting", "dealer_accounting"],
    });
    expect(on.canWrite).toBe(true);
    expect(on.readOnlyDealer).toBe(false);
  });

  it("disputing needs accounting.dispute and a parent organization", () => {
    const granted = [Permission.AccountingRead, Permission.AccountingDispute];
    expect(
      resolveAccountingAccess({
        can: can(granted),
        orgType: "dealer",
        features: [],
        parentUuid: "dist-1",
      }).canDispute,
    ).toBe(true);
    // The center has no parent to dispute against.
    expect(
      resolveAccountingAccess({
        can: can(granted),
        orgType: "center",
        parentUuid: null,
      }).canDispute,
    ).toBe(false);
    expect(
      resolveAccountingAccess({
        can: can([Permission.AccountingRead]),
        orgType: "dealer",
        parentUuid: "dist-1",
      }).canDispute,
    ).toBe(false);
  });

  it("resolving needs accounting.resolve on a center or distributor", () => {
    const granted = [Permission.AccountingRead, Permission.AccountingResolve];
    for (const orgType of ["center", "distributor"]) {
      expect(
        resolveAccountingAccess({ can: can(granted), orgType }).canResolve,
      ).toBe(true);
    }
    expect(
      resolveAccountingAccess({ can: can(granted), orgType: "dealer" })
        .canResolve,
    ).toBe(false);
    expect(
      resolveAccountingAccess({
        can: can([Permission.AccountingRead]),
        orgType: "distributor",
      }).canResolve,
    ).toBe(false);
  });
});
