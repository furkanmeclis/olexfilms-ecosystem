import type { AppointmentOccupancy } from "@/features/appointments/services/appointments.service";

/**
 * Heat levels of the network occupancy view (TEC-328). The thresholds are
 * defined only here: a rate at or above `full` is full, at or above `high`
 * busy, at or above `medium` medium, anything lower low; no capacity is
 * "none" (the organization has no appointment settings).
 */
export const OCCUPANCY_THRESHOLDS = {
  medium: 0.5,
  high: 0.8,
  full: 1,
} as const;

export type OccupancyLevel = "none" | "low" | "medium" | "high" | "full";

export const OCCUPANCY_LEVELS: readonly OccupancyLevel[] = [
  "low",
  "medium",
  "high",
  "full",
  "none",
];

/** Badge / cell tone per level (light and dark). */
export const OCCUPANCY_TONE: Record<OccupancyLevel, string> = {
  none: "bg-muted text-muted-foreground border-border",
  low: "bg-emerald-100 text-emerald-900 border-emerald-300 dark:bg-emerald-950 dark:text-emerald-100 dark:border-emerald-800",
  medium:
    "bg-amber-100 text-amber-900 border-amber-300 dark:bg-amber-950 dark:text-amber-100 dark:border-amber-800",
  high: "bg-orange-200 text-orange-950 border-orange-400 dark:bg-orange-950 dark:text-orange-100 dark:border-orange-700",
  full: "bg-red-200 text-red-950 border-red-400 dark:bg-red-950 dark:text-red-100 dark:border-red-700",
};

/** occupied / capacity, or null without capacity. */
export function occupancyRate(
  occupied: number,
  capacity: number,
): number | null {
  if (!(capacity > 0)) return null;
  return Math.max(occupied, 0) / capacity;
}

export function occupancyLevel(rate: number | null): OccupancyLevel {
  if (rate === null) return "none";
  if (rate >= OCCUPANCY_THRESHOLDS.full) return "full";
  if (rate >= OCCUPANCY_THRESHOLDS.high) return "high";
  if (rate >= OCCUPANCY_THRESHOLDS.medium) return "medium";
  return "low";
}

/** Whole percent (rounded) of a rate; null stays null. */
export function occupancyPercent(rate: number | null): number | null {
  return rate === null ? null : Math.round(rate * 100);
}

export type OccupancyRow = {
  organization_uuid: string;
  organization_name: string;
  capacity: number;
  occupied: number;
  remaining: number;
  rate: number | null;
  level: OccupancyLevel;
};

/**
 * Sums the per-day occupancy of each organization over the given days
 * (one day: as is; a week: capacity and bookings of the seven days).
 * Order follows the first day's rows.
 */
export function aggregateOccupancy(
  days: readonly (readonly AppointmentOccupancy[])[],
): OccupancyRow[] {
  const byOrg = new Map<string, OccupancyRow>();
  for (const rows of days) {
    for (const r of rows) {
      const cur = byOrg.get(r.organization_uuid);
      if (cur) {
        cur.capacity += r.capacity;
        cur.occupied += r.occupied;
        cur.remaining += r.remaining;
      } else {
        byOrg.set(r.organization_uuid, {
          organization_uuid: r.organization_uuid,
          organization_name: r.organization_name,
          capacity: r.capacity,
          occupied: r.occupied,
          remaining: r.remaining,
          rate: null,
          level: "none",
        });
      }
    }
  }
  return [...byOrg.values()].map((row) => {
    const rate = occupancyRate(row.occupied, row.capacity);
    return { ...row, rate, level: occupancyLevel(rate) };
  });
}

export type OccupancySummary = {
  organizations: number;
  capacity: number;
  occupied: number;
  remaining: number;
  rate: number | null;
  level: OccupancyLevel;
  /** Organizations per level (dashboard breakdown). */
  levels: Record<OccupancyLevel, number>;
};

/** Network totals of aggregated rows (dashboard widget). */
export function summarizeOccupancy(
  rows: readonly OccupancyRow[],
): OccupancySummary {
  const levels: Record<OccupancyLevel, number> = {
    none: 0,
    low: 0,
    medium: 0,
    high: 0,
    full: 0,
  };
  let capacity = 0;
  let occupied = 0;
  let remaining = 0;
  for (const r of rows) {
    capacity += r.capacity;
    occupied += r.occupied;
    remaining += r.remaining;
    levels[r.level] += 1;
  }
  const rate = occupancyRate(occupied, capacity);
  return {
    organizations: rows.length,
    capacity,
    occupied,
    remaining,
    rate,
    level: occupancyLevel(rate),
    levels,
  };
}
