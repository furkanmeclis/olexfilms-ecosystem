const DAYS_PER_MONTH = 365.25 / 12;

/** Whole years and months in `days`; months stay within 0..11. */
export function splitTimeLeft(days: number) {
  const total = Math.floor(days / DAYS_PER_MONTH);
  return { years: Math.floor(total / 12), months: total % 12 };
}
