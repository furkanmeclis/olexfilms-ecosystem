import { describe, expect, it } from "vitest";

import {
  canContinueWizard,
  canDownloadWarrantyCertificate,
  resolveServiceListAccess,
  resolveServiceWizardAccess,
} from "./access";

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

describe("service list / detail access (TEC-183)", () => {
  const wizard = ["services.write", "customers.read", "vehicles.read"];

  it("reads with services.read, creates with the wizard grants", () => {
    expect(resolveServiceListAccess(canOf([]))).toEqual({
      canRead: false,
      canCreate: false,
    });
    expect(resolveServiceListAccess(canOf(["services.read"]))).toEqual({
      canRead: true,
      canCreate: false,
    });
    expect(
      resolveServiceListAccess(canOf(["services.read", ...wizard])),
    ).toEqual({ canRead: true, canCreate: true });
  });

  it("continues only an editable draft with the wizard grants", () => {
    const draft = { status: "draft", items_editable: true };
    expect(canContinueWizard(canOf(wizard), draft)).toBe(true);
    expect(canContinueWizard(canOf(["services.read"]), draft)).toBe(false);
    expect(
      canContinueWizard(canOf(wizard), { ...draft, items_editable: false }),
    ).toBe(false);
    expect(
      canContinueWizard(canOf(wizard), { ...draft, status: "pending" }),
    ).toBe(false);
  });
});

describe("canDownloadWarrantyCertificate", () => {
  const active = { status: "completed", warranties: [{ status: "active" }] };
  it("needs warranties.read and a completed service with an active warranty", () => {
    expect(
      canDownloadWarrantyCertificate(canOf(["warranties.read"]), active),
    ).toBe(true);
    expect(canDownloadWarrantyCertificate(canOf([]), active)).toBe(false);
    expect(
      canDownloadWarrantyCertificate(canOf(["warranties.read"]), {
        status: "completed",
        warranties: [{ status: "expired" }, { status: "void" }],
      }),
    ).toBe(false);
    expect(
      canDownloadWarrantyCertificate(canOf(["warranties.read"]), {
        status: "ready",
        warranties: [],
      }),
    ).toBe(false);
  });
});
