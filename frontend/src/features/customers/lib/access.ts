import { permissions } from "@/config/permissions";
import type { CustomerDetail } from "@/features/customers/services/customers.service";
import type { OrganizationType } from "@/lib/auth/types";

type Can = (permission: string) => boolean;

export type CustomerListAccess = {
  /** GET /v1/customers needs customers.read. */
  canRead: boolean;
  /** POST /v1/customers needs customers.write. */
  canCreate: boolean;
  /** POST /v1/customers/export (TEC-164) needs customers.read. */
  canExport: boolean;
};

export function resolveCustomerListAccess(can: Can): CustomerListAccess {
  const read = can(permissions.customers.read);
  return {
    canRead: read,
    canCreate: can(permissions.customers.write),
    canExport: read,
  };
}

export type CustomerDetailAccess = {
  /** PATCH /v1/customers/{uuid}: customers.write and an editable account. */
  canEdit: boolean;
  /** The vehicle list (vehicles.read). */
  canReadVehicles: boolean;
  /** Vehicle create / edit: vehicles.write on an editable account. */
  canWriteVehicles: boolean;
  /**
   * Anonymization (K19): customers.anonymize (center roles) in a center
   * organization; the backend also needs a fresh step-up. Hidden once the
   * customer is anonymized.
   */
  canAnonymize: boolean;
  /** Personal data export: same gate as anonymization (TEC-161). */
  canExport: boolean;
  /**
   * Upgrade to dealer: customers.write in a center or distributor
   * organization (dealers get 403) plus organizations.read for the dealer
   * picker; not for anonymized or inactive accounts.
   */
  canUpgrade: boolean;
};

/**
 * Buttons on the customer detail, mirroring the API gates so a user never
 * sees an action the backend would refuse with 403.
 */
export function resolveCustomerDetailAccess(
  can: Can,
  orgType: OrganizationType | null | undefined,
  customer: Pick<CustomerDetail, "editable" | "anonymized" | "status">,
): CustomerDetailAccess {
  const center = orgType === "center";
  const privacy = can(permissions.customers.anonymize) && center;
  const live = !customer.anonymized && customer.status !== "anonymized";
  return {
    canEdit: can(permissions.customers.write) && customer.editable,
    canReadVehicles: can(permissions.vehicles.read),
    canWriteVehicles:
      can(permissions.vehicles.read) &&
      can(permissions.vehicles.write) &&
      customer.editable,
    canAnonymize: privacy && live,
    canExport: privacy && live,
    canUpgrade:
      can(permissions.customers.write) &&
      can(permissions.organizations.tenantRead) &&
      (center || orgType === "distributor") &&
      live &&
      customer.status === "active",
  };
}
