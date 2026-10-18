/**
 * Customer appointment booking (web `portal-appointment-booking`): pure rules
 * for the day window, dealer list and time labels.
 */

/** Days shown at once in the day picker. */
export const BOOKING_WINDOW_DAYS = 14;
/** How far ahead the day picker can go (the API range limit is 62). */
export const BOOKING_MAX_AHEAD_DAYS = 56;
/** Search radii of the dealer list (km); the widest covers the country. */
export const RADIUS_OPTIONS = [25, 50, 100, 250, 500, 1000] as const;
export type RadiusKm = (typeof RADIUS_OPTIONS)[number];
/** No device position (expo-location is not installed): centre of the country. */
export const FALLBACK_CENTER = { lat: 39.0, lng: 35.0 } as const;
export const FALLBACK_RADIUS_KM: RadiusKm = 1000;

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

export type DealerLike = { name: string; city: string; district: string };

/** Client-side text filter over the nearby list (the API only searches by position). */
export function filterDealers<T extends DealerLike>(dealers: T[], query: string): T[] {
  const q = query.trim().toLocaleLowerCase();
  if (!q) return dealers;
  return dealers.filter((d) => [d.name, d.city, d.district].some((s) => s.toLocaleLowerCase().includes(q)));
}

export function formatDistanceKm(km: number, locale: string): string {
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: km < 10 ? 1 : 0 }).format(km)} km`;
}
