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

export type ServiceListAccess = {
  /** The list and detail open (GET /v1/services needs services.read). */
  canRead: boolean;
  /** "New service" opens the wizard. */
  canCreate: boolean;
};

export function resolveServiceListAccess(can: Can): ServiceListAccess {
  return {
    canRead: can(permissions.services.read),
    canCreate: resolveServiceWizardAccess(can).canStart,
  };
}

/**
 * "Continue in wizard" on the detail page: a draft whose items the caller
 * may still change, and the wizard itself opens for the caller.
 */
export function canContinueWizard(
  can: Can,
  service: { status: string; items_editable: boolean },
): boolean {
  return (
    service.status === "draft" &&
    service.items_editable &&
    resolveServiceWizardAccess(can).canStart
  );
}

/**
 * "Warranty PDF" on the detail page (TEC-188): warranties.read and a
 * completed service with at least one active warranty (the API answers 409
 * NO_ACTIVE_WARRANTY otherwise).
 */
export function canDownloadWarrantyCertificate(
  can: Can,
  service: {
    status: string;
    warranties?: readonly { status: string }[] | null;
  },
): boolean {
  return (
    can(permissions.warranties.read) &&
    service.status === "completed" &&
    (service.warranties ?? []).some((w) => w.status === "active")
  );
}

export type ServiceIncomeAccess = {
  /** "Record income": a completed service without income yet. */
  canRecord: boolean;
  /** "Reverse income": the recorded income of the service. */
  canReverse: boolean;
};

/**
 * Service income (TEC-343): the backend needs accounting.write on the
 * service organization (a dealer also needs dealer_accounting, which
 * `accountingWrite` already folds in) and books into that organization's
 * own accounts, so only the service organization itself records it.
 */
export function resolveServiceIncomeAccess(
  service: {
    status: string;
    organization: { uuid: string };
    income_amount?: string | null;
  },
  accounting: { canWrite: boolean; orgUuid: string },
): ServiceIncomeAccess {
  const own =
    accounting.canWrite &&
    Boolean(accounting.orgUuid) &&
    service.organization.uuid === accounting.orgUuid;
  const recorded =
    service.income_amount !== null && service.income_amount !== undefined;
  return {
    canRecord: own && service.status === "completed" && !recorded,
    canReverse: own && recorded,
  };
}
