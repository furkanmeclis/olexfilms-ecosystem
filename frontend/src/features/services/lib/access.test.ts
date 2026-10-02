import { describe, expect, it } from "vitest";

import { resolveServiceWizardAccess } from "./access";

const canOf = (grants: string[]) => (p: string) => grants.includes(p);

describe("resolveServiceWizardAccess", () => {
  it("needs services.write plus customer and vehicle reads", () => {
    expect(resolveServiceWizardAccess(canOf(["services.write"])).canStart).toBe(
      false,
    );
    expect(
      resolveServiceWizardAccess(
        canOf(["services.write", "customers.read", "vehicles.read"]),
      ),
    ).toEqual({
      canStart: true,
      canCreateCustomer: false,
      canCreateVehicle: false,
    });
  });

  it("unlocks the create forms with the write grants", () => {
    expect(
      resolveServiceWizardAccess(
        canOf([
          "services.write",
          "customers.read",
          "customers.write",
          "vehicles.read",
          "vehicles.write",
        ]),
      ),
    ).toEqual({
      canStart: true,
      canCreateCustomer: true,
      canCreateVehicle: true,
    });
  });

  it("write grants alone do not open the wizard", () => {
    expect(
      resolveServiceWizardAccess(canOf(["customers.write", "vehicles.write"])),
    ).toEqual({
      canStart: false,
      canCreateCustomer: false,
      canCreateVehicle: false,
    });
  });
});
