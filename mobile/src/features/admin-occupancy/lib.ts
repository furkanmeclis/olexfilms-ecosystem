/**
 * Heat levels of the network occupancy view, same thresholds as the web
 * (features/appointments/lib/occupancy.ts): full at 100%, busy from 80%,
 * medium from 50%; no capacity means the organization has no appointment settings.
 */
export type OccupancyLevel = 'none' | 'low' | 'medium' | 'high' | 'full';

export type OccupancyInput = {
  organization_uuid: string;
  organization_name: string;
  capacity: number;
  occupied: number;
  remaining: number;
};

export type OccupancyRow = OccupancyInput & { rate: number | null; level: OccupancyLevel };

export function occupancyRate(occupied: number, capacity: number): number | null {
  if (!(capacity > 0)) return null;
  return Math.max(occupied, 0) / capacity;
}

export function occupancyLevel(rate: number | null): OccupancyLevel {
  if (rate === null) return 'none';
  if (rate >= 1) return 'full';
  if (rate >= 0.8) return 'high';
  if (rate >= 0.5) return 'medium';
  return 'low';
}

/** Whole percent of a rate; null stays null. */
export const occupancyPercent = (rate: number | null) => (rate === null ? null : Math.round(rate * 100));

/** Sums each organization over the given days (one day as is, a week is seven days). */
export function aggregateOccupancy(days: readonly (readonly OccupancyInput[])[]): OccupancyRow[] {
  const byOrg = new Map<string, OccupancyInput>();
  for (const rows of days) {
    for (const r of rows) {
      const cur = byOrg.get(r.organization_uuid);
      if (cur) {
        cur.capacity += r.capacity;
        cur.occupied += r.occupied;
        cur.remaining += r.remaining;
      } else {
        byOrg.set(r.organization_uuid, { ...r });
      }
    }
  }
  return [...byOrg.values()]
    .map((row) => {
      const rate = occupancyRate(row.occupied, row.capacity);
      return { ...row, rate, level: occupancyLevel(rate) };
    })
    .sort((a, b) => (b.rate ?? -1) - (a.rate ?? -1) || a.organization_name.localeCompare(b.organization_name));
}

export function summarizeOccupancy(rows: readonly OccupancyRow[]) {
  let capacity = 0;
  let occupied = 0;
  for (const r of rows) {
    capacity += r.capacity;
    occupied += r.occupied;
  }
  const rate = occupancyRate(occupied, capacity);
  return { organizations: rows.length, capacity, occupied, rate, level: occupancyLevel(rate) };
}
