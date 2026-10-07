import type {
  CancelRequestStatus,
  TargetOrganization,
  ServiceSubscription,
  ServiceSubscriptionStatus,
} from "@/features/service-subscriptions/services/service-subscriptions.service";

type Tone = "default" | "success" | "warning" | "danger";

export function subscriptionStatusTone(
  status: ServiceSubscriptionStatus,
): Tone {
  switch (status) {
    case "active":
      return "success";
    case "cancel_requested":
      return "warning";
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}

export function cancelRequestStatusTone(status: CancelRequestStatus): Tone {
  switch (status) {
    case "pending":
      return "warning";
    case "approved":
      return "success";
    default:
      return "danger";
  }
}

export function amountNumber(value?: string | null): number | null {
  if (!value) return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

/** Only center and distributor assign (dealers have no assign grant). */
export function canAssignFrom(orgType: string | undefined): boolean {
  return orgType === "center" || orgType === "distributor";
}

/**
 * Early cancellation is requested by the subscriber organization for its
 * own active subscription (the API answers 404 / 409 otherwise).
 */
export function canRequestCancel(
  sub: Pick<ServiceSubscription, "organization_uuid" | "status">,
  activeOrgUuid: string | undefined,
  hasPermission: boolean,
): boolean {
  return (
    hasPermission &&
    Boolean(activeOrgUuid) &&
    sub.organization_uuid === activeOrgUuid &&
    sub.status === "active"
  );
}

export type AssignForm = {
  organizationUuid: string;
  itemUuid: string;
  startsOn: string;
  endsOn: string;
};

/** Every field is required and the end date must follow the start. */
export function assignFormReady(form: AssignForm): boolean {
  if (!form.organizationUuid || !form.itemUuid) return false;
  if (!form.startsOn || !form.endsOn) return false;
  return form.endsOn > form.startsOn;
}

/**
 * Assignment targets: the center assigns to distributors and dealers, a
 * distributor to its own dealers (its scope holds only its subtree).
 */
export function assignTargets(
  orgs: TargetOrganization[],
  orgType: string | undefined,
): TargetOrganization[] {
  const allowed =
    orgType === "center"
      ? ["distributor", "dealer"]
      : orgType === "distributor"
        ? ["dealer"]
        : [];
  return orgs.filter((o) => allowed.includes(o.type));
}
