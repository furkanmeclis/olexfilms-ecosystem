import { permissions } from "@/config/permissions";

type Can = (permission: string) => boolean;

export type ServiceWizardAccess = {
  /**
   * The wizard opens: services.write (POST /v1/services) plus reading the
   * customers and their vehicles it picks from (TEC-160 routes).
   */
  canStart: boolean;
  /** "New customer" form (customers.write). */
  canCreateCustomer: boolean;
  /** "New vehicle" form (vehicles.write). */
  canCreateVehicle: boolean;
};

/** Mirrors the API gates of /v1/services, /v1/customers and /v1/vehicles. */
export function resolveServiceWizardAccess(can: Can): ServiceWizardAccess {
  const canStart =
    can(permissions.services.write) &&
    can(permissions.customers.read) &&
    can(permissions.vehicles.read);
  return {
    canStart,
    canCreateCustomer: canStart && can(permissions.customers.write),
    canCreateVehicle: canStart && can(permissions.vehicles.write),
  };
}
