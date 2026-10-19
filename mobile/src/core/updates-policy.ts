/** Minimum gap between two update checks triggered by app foregrounding. */
export const UPDATE_CHECK_INTERVAL_MS = 15 * 60 * 1000;

/** True when enough time passed since the last check (or none happened yet). */
export function shouldCheckForUpdate(
  lastCheckedAt: number | null,
  now: number,
  intervalMs = UPDATE_CHECK_INTERVAL_MS,
): boolean {
  return lastCheckedAt === null || now - lastCheckedAt >= intervalMs;
}
