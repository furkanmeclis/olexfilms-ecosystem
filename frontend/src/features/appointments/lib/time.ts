/**
 * Calendar time math in an IANA zone (TEC-326). The calendar renders in the
 * organization's zone, not the browser's, so every conversion goes through
 * Intl. Calendar days are plain `YYYY-MM-DD` strings; minutes count from the
 * zone's local midnight.
 */

const partsFormatters = new Map<string, Intl.DateTimeFormat>();

function partsFormatter(timeZone: string): Intl.DateTimeFormat {
  let f = partsFormatters.get(timeZone);
  if (!f) {
    f = new Intl.DateTimeFormat("en-CA", {
      timeZone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hourCycle: "h23",
    });
    partsFormatters.set(timeZone, f);
  }
  return f;
}

type WallTime = {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
};

function wallTime(instant: Date, timeZone: string): WallTime {
  const out: Record<string, number> = {};
  for (const p of partsFormatter(timeZone).formatToParts(instant)) {
    if (p.type !== "literal") out[p.type] = Number(p.value);
  }
  return {
    year: out.year ?? 1970,
    month: out.month ?? 1,
    day: out.day ?? 1,
    // Some engines print midnight as 24 even with h23.
    hour: (out.hour ?? 0) % 24,
    minute: out.minute ?? 0,
    second: out.second ?? 0,
  };
}

const pad = (n: number) => String(n).padStart(2, "0");

function dateKey(year: number, month: number, day: number): string {
  return `${year}-${pad(month)}-${pad(day)}`;
}

function parseDateKey(date: string): [number, number, number] {
  const [y, m, d] = date.split("-").map(Number);
  return [y ?? 1970, m ?? 1, d ?? 1];
}

/** Calendar day and minutes after local midnight of an instant in a zone. */
export function zonedParts(
  value: string | Date,
  timeZone: string,
): { date: string; minutes: number } {
  const w = wallTime(new Date(value), timeZone);
  return {
    date: dateKey(w.year, w.month, w.day),
    minutes: w.hour * 60 + w.minute,
  };
}

/** Offset of the zone from UTC at an instant, in milliseconds. */
function offsetMs(instant: number, timeZone: string): number {
  const w = wallTime(new Date(instant), timeZone);
  const asUtc = Date.UTC(
    w.year,
    w.month - 1,
    w.day,
    w.hour,
    w.minute,
    w.second,
  );
  return asUtc - Math.floor(instant / 1000) * 1000;
}

/**
 * The instant of a local wall time (day + minutes) in a zone, as an RFC3339
 * UTC string. A wall time skipped by a DST jump resolves forward.
 */
export function zonedToUtc(
  date: string,
  minutes: number,
  timeZone: string,
): string {
  const [y, m, d] = parseDateKey(date);
  const guess = Date.UTC(y, m - 1, d, 0, minutes);
  let instant = guess - offsetMs(guess, timeZone);
  const second = guess - offsetMs(instant, timeZone);
  if (second !== instant) instant = second;
  return new Date(instant).toISOString().replace(".000Z", "Z");
}

export function addDays(date: string, days: number): string {
  const [y, m, d] = parseDateKey(date);
  const t = new Date(Date.UTC(y, m - 1, d + days));
  return dateKey(t.getUTCFullYear(), t.getUTCMonth() + 1, t.getUTCDate());
}

/** 0 = Sunday … 6 = Saturday. */
export function weekdayOf(date: string): number {
  const [y, m, d] = parseDateKey(date);
  return new Date(Date.UTC(y, m - 1, d)).getUTCDay();
}

/** The seven days of the (Monday-first) week holding `date`. */
export function weekDays(date: string): string[] {
  const start = addDays(date, -((weekdayOf(date) + 6) % 7));
  return Array.from({ length: 7 }, (_, i) => addDays(start, i));
}

/** Today in a zone. */
export function todayIn(timeZone: string, now: Date = new Date()): string {
  return zonedParts(now, timeZone).date;
}

/** [from, to) instants covering whole local days, for GET /v1/appointments. */
export function daysRange(
  days: string[],
  timeZone: string,
): { from: string; to: string } {
  const first = days[0] ?? todayIn(timeZone);
  const last = days[days.length - 1] ?? first;
  return {
    from: zonedToUtc(first, 0, timeZone),
    to: zonedToUtc(addDays(last, 1), 0, timeZone),
  };
}

/** "HH:MM" of minutes after midnight. */
export function minutesLabel(minutes: number): string {
  return `${pad(Math.floor(minutes / 60))}:${pad(minutes % 60)}`;
}

/** Minutes after midnight of "HH:MM", or null when malformed. */
export function parseClock(value: string): number | null {
  const m = /^(\d{2}):(\d{2})$/.exec(value.trim());
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  if (h > 23 || min > 59) return null;
  return h * 60 + min;
}
