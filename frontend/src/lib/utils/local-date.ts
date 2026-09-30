/** Local calendar date (yyyy-MM-dd); UTC would show yesterday after midnight in TR. */
export function localToday(): string {
  return toISODate(new Date());
}

export function shiftDate(isoDate: string, days: number): string {
  const [y, m, d] = isoDate.split("-").map(Number);
  return toISODate(new Date(y, m - 1, d + days));
}

/** Local calendar date (yyyy-MM-dd) of an ISO timestamp. */
export function localDateOf(iso: string): string {
  return toISODate(new Date(iso));
}

/** Whole calendar days from `from` to `to` (both yyyy-MM-dd). */
export function calendarDaysBetween(from: string, to: string): number {
  const [fy, fm, fd] = from.split("-").map(Number);
  const [ty, tm, td] = to.split("-").map(Number);
  return Math.round(
    (Date.UTC(ty, tm - 1, td) - Date.UTC(fy, fm - 1, fd)) / 86_400_000,
  );
}

function toISODate(date: Date): string {
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${m}-${d}`;
}
