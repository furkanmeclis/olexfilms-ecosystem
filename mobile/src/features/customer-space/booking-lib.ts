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

/** Same default centre and radius as the web's dealer finder (frontend/src/config/map.ts). */
export const PUBLIC_DEALER_CENTER = { lat: 39.0, lng: 35.0 } as const;
export const PUBLIC_DEALER_RADIUS_KM = 1000;

/** The fields of a public "find a dealer" row the booking flow uses. */
export type PublicDealer = {
  uuid: string;
  name: string;
  city: string;
  district: string;
  accepts_appointments: boolean;
};

/** A dealer the booking flow can pick; the place is shown under the name. */
export type BookingDealer = { uuid: string; name: string; place?: string };

/** Turkish-aware lowercase without accents, so "istanbul" finds "İstanbul" and "sisli" finds "Şişli". */
export function foldSearch(s: string): string {
  let lower: string;
  try {
    lower = s.toLocaleLowerCase('tr');
  } catch {
    lower = s.toLowerCase();
  }
  return lower
    .replace(/ı/g, 'i')
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .trim();
}

/** "District, City" (either part may be empty). */
export function dealerPlace(d: { city: string; district: string }): string {
  return [d.district, d.city].filter(Boolean).join(', ');
}

/** Dealers that take bookings, matching the text on name, city or district. */
export function filterPublicDealers(items: readonly PublicDealer[], query: string): BookingDealer[] {
  const q = foldSearch(query);
  const out: BookingDealer[] = [];
  for (const d of items) {
    if (d.accepts_appointments !== true) continue;
    if (q && ![d.name, d.city, d.district].some((v) => foldSearch(v ?? '').includes(q))) continue;
    out.push({ uuid: d.uuid, name: d.name, place: dealerPlace(d) || undefined });
  }
  return out;
}

/** Past dealers matching the text on name (or place when known). */
export function filterPastDealers(items: readonly BookingDealer[], query: string): BookingDealer[] {
  const q = foldSearch(query);
  if (!q) return [...items];
  return items.filter((d) => foldSearch(`${d.name} ${d.place ?? ''}`).includes(q));
}

/** Public list minus the dealers already shown as past dealers. */
export function dedupeDealers(past: readonly BookingDealer[], all: readonly BookingDealer[]): BookingDealer[] {
  const seen = new Set(past.map((d) => d.uuid));
  return all.filter((d) => !seen.has(d.uuid));
}

/** Retry-After in seconds (delta form only); null when missing or unusable. */
export function parseRetryAfter(value: string | null | undefined): number | null {
  if (!value) return null;
  const n = Number(value.trim());
  return Number.isFinite(n) && n >= 0 ? n : null;
}

/** Past dealers get their place (district, city) from the matching public row, when it is in the list. */
export function enrichPastDealers(past: readonly BookingDealer[], publicRows: readonly BookingDealer[]): BookingDealer[] {
  const places = new Map(publicRows.map((d) => [d.uuid, d.place]));
  return past.map((d) => ({ ...d, place: d.place ?? places.get(d.uuid) }));
}

/** True when the public list came back full: bookable dealers may be missing from it. */
export function publicListCapped(count: number, cap: number): boolean {
  return count >= cap;
}
