import type { ReportQuery } from "@/features/reports";

export type DateRange = { from: string; to: string };

export type ComparedRange = DateRange & { previous: DateRange | null };

function iso(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function parse(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1);
}

function addDays(d: Date, n: number): Date {
  const out = new Date(d);
  out.setDate(out.getDate() + n);
  return out;
}

/**
 * TEC-1212: calendar range of a PeriodBar query for endpoints that take
 * from / to (accounting reports), with the compared window: same rules as
 * the reports module (TEC-1210): presets end today (12m starts on the
 * first day of the month eleven months back), "previous" is the same
 * length right before, "year" the same dates a year earlier.
 */
export function periodRange(query: ReportQuery, now: Date): ComparedRange {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  let from: Date;
  let to = today;
  const rf = typeof query.range_from === "string" ? query.range_from : "";
  const rt = typeof query.range_to === "string" ? query.range_to : "";
  if (rf && rt) {
    from = parse(rf);
    to = parse(rt);
  } else {
    switch (query.period) {
      case "today":
        from = today;
        break;
      case "7d":
        from = addDays(today, -6);
        break;
      case "90d":
        from = addDays(today, -89);
        break;
      case "12m":
        from = new Date(today.getFullYear(), today.getMonth() - 11, 1);
        break;
      default:
        from = addDays(today, -29);
    }
  }
  let previous: DateRange | null = null;
  if (query.compare === "previous") {
    const days = Math.round((to.getTime() - from.getTime()) / 86_400_000) + 1;
    const pTo = addDays(from, -1);
    previous = { from: iso(addDays(pTo, -(days - 1))), to: iso(pTo) };
  } else if (query.compare === "year") {
    const shift = (d: Date) =>
      new Date(d.getFullYear() - 1, d.getMonth(), d.getDate());
    previous = { from: iso(shift(from)), to: iso(shift(to)) };
  }
  return { from: iso(from), to: iso(to), previous };
}
