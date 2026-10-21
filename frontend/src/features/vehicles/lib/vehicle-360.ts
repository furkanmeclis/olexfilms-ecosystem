/**
 * Vehicle 360 view helpers (TEC-863): the summary figures of the vehicle
 * page, computed from the vehicle's services, warranties and warranty
 * claims as the scoped list endpoints return them (`vehicle_uuid` filter).
 */

/** Rows fetched per section; a vehicle rarely has more. */
export const VEHICLE_360_LIMIT = 100;

type ServiceLike = {
  status: string;
  created_at: string;
  completed_at?: string | null;
  customer?: {
    uuid: string;
    name: string;
    surname: string;
    phone: string | null;
    anonymized: boolean;
  } | null;
};

type WarrantyLike = { status: string; end_at: string };
type ClaimLike = { status: string };

export type Vehicle360Summary = {
  /** Non-cancelled services. */
  serviceCount: number;
  /** Latest completion (or opening, while not completed) of a live service. */
  lastServiceAt: string | null;
  activeWarranties: number;
  /** Soonest end of an active warranty. */
  nextWarrantyEnd: string | null;
  /** Claims not rejected or closed. */
  openClaims: number;
};

const time = (v: string | null | undefined) => {
  if (!v) return Number.NaN;
  return Date.parse(v);
};

/** The date a service counts as "done" for the last service KPI. */
export function serviceDate(s: ServiceLike): string {
  return s.completed_at || s.created_at;
}

export function summarizeVehicle360(input: {
  services: readonly ServiceLike[];
  warranties: readonly WarrantyLike[];
  claims: readonly ClaimLike[];
}): Vehicle360Summary {
  let serviceCount = 0;
  let lastServiceAt: string | null = null;
  for (const s of input.services) {
    if (s.status === "cancelled") continue;
    serviceCount += 1;
    const at = serviceDate(s);
    if (
      Number.isFinite(time(at)) &&
      (lastServiceAt === null || time(at) > time(lastServiceAt))
    ) {
      lastServiceAt = at;
    }
  }

  let activeWarranties = 0;
  let nextWarrantyEnd: string | null = null;
  for (const w of input.warranties) {
    if (w.status !== "active") continue;
    activeWarranties += 1;
    if (
      Number.isFinite(time(w.end_at)) &&
      (nextWarrantyEnd === null || time(w.end_at) < time(nextWarrantyEnd))
    ) {
      nextWarrantyEnd = w.end_at;
    }
  }

  const openClaims = input.claims.filter(
    (c) => c.status !== "rejected" && c.status !== "closed",
  ).length;

  return {
    serviceCount,
    lastServiceAt,
    activeWarranties,
    nextWarrantyEnd,
    openClaims,
  };
}

/**
 * Owner shown when the customer record cannot be read (no customers.read):
 * the customer of the newest service, the vehicle's owner at that time.
 * Only a service of the vehicle's current customer counts.
 */
export function ownerFromServices<S extends ServiceLike>(
  services: readonly S[],
  customerUuid: string,
): NonNullable<S["customer"]> | null {
  let best: S | null = null;
  for (const s of services) {
    if (!s.customer || s.customer.uuid !== customerUuid) continue;
    if (best === null || time(s.created_at) > time(best.created_at)) best = s;
  }
  return (best?.customer as NonNullable<S["customer"]> | undefined) ?? null;
}

/** Display name of a person with a name and an optional surname. */
export function personName(
  p: { name?: string | null; surname?: string | null } | null | undefined,
): string {
  if (!p) return "";
  return [p.name, p.surname].filter(Boolean).join(" ");
}
