import { minutesLabel, parseClock } from "./time";

/** Monday-first weekday keys of the working_hours object (backend keys). */
export const WEEKDAYS = [
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
  "sunday",
] as const;

export type Weekday = (typeof WEEKDAYS)[number];

export type HoursWindow = { start: string; end: string };

/**
 * A weekday of the form: its opening windows in saved order. A closed day
 * keeps its windows so switching it back on restores them.
 */
export type DayHours = { open: boolean; windows: HoursWindow[] };

export type WorkingHoursForm = Record<Weekday, DayHours>;

const DEFAULT_WINDOW: HoursWindow = { start: "09:00", end: "18:00" };

type RawWindow = { start?: unknown; end?: unknown };

/** Keys the backend accepts for a weekday: full, short and ISO number. */
function keysOf(day: Weekday, index: number): string[] {
  return [day, day.slice(0, 3), String(index + 1)];
}

/**
 * The form model of a working_hours object. A day is open when it has a
 * window; every saved window is kept so saving round-trips them.
 */
export function workingHoursFromApi(raw: unknown): WorkingHoursForm {
  const src = (raw && typeof raw === "object" ? raw : {}) as Record<
    string,
    unknown
  >;
  const out = {} as WorkingHoursForm;
  WEEKDAYS.forEach((day, i) => {
    const key = keysOf(day, i).find((k) => Array.isArray(src[k]));
    const windows = (key ? (src[key] as RawWindow[]) : [])
      .filter(
        (w): w is HoursWindow =>
          !!w && typeof w.start === "string" && typeof w.end === "string",
      )
      .map((w) => ({ start: w.start, end: w.end }));
    out[day] = windows.length
      ? { open: true, windows }
      : { open: false, windows: [{ ...DEFAULT_WINDOW }] };
  });
  return out;
}

/** The working_hours object of the form: open days only, full keys. */
export function workingHoursToApi(
  form: WorkingHoursForm,
): Record<string, HoursWindow[]> {
  const out: Record<string, HoursWindow[]> = {};
  for (const day of WEEKDAYS) {
    const h = form[day];
    if (h.open && h.windows.length) {
      out[day] = h.windows.map((w) => ({ start: w.start, end: w.end }));
    }
  }
  return out;
}

/**
 * The window a day gains from "add window": an hour starting where the
 * last window closes, or the default hours when that does not fit.
 */
export function nextWindow(windows: HoursWindow[]): HoursWindow {
  const last = windows.length ? parseClock(windows.at(-1)!.end) : null;
  if (last === null || last + 60 > 23 * 60 + 59) return { ...DEFAULT_WINDOW };
  return { start: minutesLabel(last), end: minutesLabel(last + 60) };
}

/** i18n keys of a day's errors: per window index, and for the whole day. */
export type DayErrors = {
  windows?: Partial<Record<number, string>>;
  day?: string;
};

export type WorkingHoursErrors = Partial<Record<Weekday, DayErrors>>;

/**
 * i18n keys of the errors of open days: malformed times or closing at or
 * before opening per window, then overlapping windows of the same day.
 */
export function validateWorkingHours(
  form: WorkingHoursForm,
): WorkingHoursErrors {
  const errors: WorkingHoursErrors = {};
  for (const day of WEEKDAYS) {
    const h = form[day];
    if (!h.open) continue;
    const windowErrors: Partial<Record<number, string>> = {};
    const ranges: [number, number][] = [];
    h.windows.forEach((w, i) => {
      const start = parseClock(w.start);
      const end = parseClock(w.end);
      if (start === null || end === null) {
        windowErrors[i] = "appointments.settings.hours_invalid";
      } else if (end <= start) {
        windowErrors[i] = "appointments.settings.close_before_open";
      } else {
        ranges.push([start, end]);
      }
    });
    if (Object.keys(windowErrors).length) {
      errors[day] = { windows: windowErrors };
      continue;
    }
    ranges.sort((a, b) => a[0] - b[0]);
    if (ranges.some((r, i) => i > 0 && r[0] < ranges[i - 1][1])) {
      errors[day] = { day: "appointments.settings.windows_overlap" };
    }
  }
  return errors;
}
