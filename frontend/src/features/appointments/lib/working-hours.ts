import { parseClock } from "./time";

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

export type DayHours = { open: boolean; start: string; end: string };

export type WorkingHoursForm = Record<Weekday, DayHours>;

const DEFAULT_HOURS: DayHours = { open: false, start: "09:00", end: "18:00" };

type Window = { start?: unknown; end?: unknown };

/** Keys the backend accepts for a weekday: full, short and ISO number. */
function keysOf(day: Weekday, index: number): string[] {
  return [day, day.slice(0, 3), String(index + 1)];
}

/**
 * The form model of a working_hours object. A day is open when it has a
 * window; the first window is edited (one opening per day in the panel).
 */
export function workingHoursFromApi(raw: unknown): WorkingHoursForm {
  const src = (raw && typeof raw === "object" ? raw : {}) as Record<
    string,
    unknown
  >;
  const out = {} as WorkingHoursForm;
  WEEKDAYS.forEach((day, i) => {
    const key = keysOf(day, i).find((k) => Array.isArray(src[k]));
    const windows = key ? (src[key] as Window[]) : [];
    const first = windows[0];
    out[day] =
      first && typeof first.start === "string" && typeof first.end === "string"
        ? { open: true, start: first.start, end: first.end }
        : { ...DEFAULT_HOURS };
  });
  return out;
}

/** The working_hours object of the form: open days only, full keys. */
export function workingHoursToApi(
  form: WorkingHoursForm,
): Record<string, { start: string; end: string }[]> {
  const out: Record<string, { start: string; end: string }[]> = {};
  for (const day of WEEKDAYS) {
    const h = form[day];
    if (h.open) out[day] = [{ start: h.start, end: h.end }];
  }
  return out;
}

export type WorkingHoursErrors = Partial<Record<Weekday, string>>;

/**
 * i18n keys of the per-day errors: malformed times, or closing at or
 * before opening.
 */
export function validateWorkingHours(
  form: WorkingHoursForm,
): WorkingHoursErrors {
  const errors: WorkingHoursErrors = {};
  for (const day of WEEKDAYS) {
    const h = form[day];
    if (!h.open) continue;
    const start = parseClock(h.start);
    const end = parseClock(h.end);
    if (start === null || end === null) {
      errors[day] = "appointments.settings.hours_invalid";
    } else if (end <= start) {
      errors[day] = "appointments.settings.close_before_open";
    }
  }
  return errors;
}
