import type { components } from '@/core/api/schema';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type FleetLink = S['FleetLink'];
export type FleetVehicle = S['FleetPortalVehicle'];
export type FleetService = S['FleetPortalService'];
export type FleetWarranty = S['FleetPortalWarranty'];
export type FleetReport = S['FleetPortalReport'];

export function fleetWarrantyTone(state: FleetWarranty['state']): StatusTone {
  if (state === 'active') return 'done';
  if (state === 'void') return 'problem';
  return 'neutral';
}

export function fleetLinkTone(status: FleetLink['status']): StatusTone {
  if (status === 'active') return 'done';
  if (status === 'pending') return 'progress';
  return 'neutral';
}

/** Dealer requests the fleet still has to answer, before the active and ended ones. */
export function sortLinks(links: readonly FleetLink[]) {
  const rank = { pending: 0, active: 1, ended: 2 } as const;
  return [...links].sort((a, b) => rank[a.status] - rank[b.status]);
}

/** `2026-07-01 – 2026-09-30` style period (ISO dates are cut to the day). */
export function reportPeriod(start: string, end: string) {
  return `${start.slice(0, 10)} – ${end.slice(0, 10)}`;
}
