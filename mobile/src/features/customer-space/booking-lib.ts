/**
 * Customer appointment booking (web `portal-appointment-booking`): pure rules
 * for the day window, dealer list and time labels.
 */

/** Days shown at once in the day picker. */
export const BOOKING_WINDOW_DAYS = 14;
/** How far ahead the day picker can go (the API range limit is 62). */
export const BOOKING_MAX_AHEAD_DAYS = 56;

export function isoDate(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${m}-${day}`;
}

/** YYYY-MM-DD plus n calendar days. */
export function addDaysIso(date: string, days: number): string {
  const [y, m, d] = date.split('-').map(Number);
  return isoDate(new Date(y, m - 1, d + days));
}

export type DayInfo = { closed: boolean; remaining_capacity: number; slots: unknown[] };
export type DayState = 'bookable' | 'closed' | 'full' | 'none';

export function dayState(day: DayInfo): DayState {
  if (day.closed) return 'closed';
  if (day.remaining_capacity <= 0) return 'full';
  return day.slots.length > 0 ? 'bookable' : 'none';
}

/** Noon UTC of a YYYY-MM-DD: the same calendar day in every time zone. */
export function dayInstant(date: string): Date {
  return new Date(`${date}T12:00:00Z`);
}

/** Start time of a slot in the dealer's zone (the device zone without one). */
export function formatSlotTime(iso: string, locale: string, timeZone?: string): string {
  const opts: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' };
  try {
    return new Intl.DateTimeFormat(locale, timeZone ? { ...opts, timeZone } : opts).format(new Date(iso));
  } catch {
    return new Intl.DateTimeFormat(locale, opts).format(new Date(iso));
  }
}

export function formatDayLabel(date: string, locale: string, options: Intl.DateTimeFormatOptions): string {
  return new Intl.DateTimeFormat(locale, { ...options, timeZone: 'UTC' }).format(dayInstant(date));
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Route params arrive as free text: only a UUID is accepted. */
export function uuidParam(v: string | string[] | undefined): string | null {
  return typeof v === 'string' && UUID_RE.test(v) ? v : null;
}

/** Today's calendar date (YYYY-MM-DD) in a zone; the device zone without one or on a bad zone. */
export function isoDateInZone(now: Date, timeZone?: string): string {
  if (timeZone) {
    try {
      const p = new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(now);
      if (/^\d{4}-\d{2}-\d{2}$/.test(p)) return p;
    } catch {
      // fall through to the device zone
    }
  }
  return isoDate(now);
}
