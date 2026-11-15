import type { Formatter } from "@/lib/i18n/format";

/**
 * Column / value types of the report envelope v2 (TEC-1210).
 * Assumptions (narrowest reading of the contract, see TEC-1211):
 * - `percent` values are percentage points (12.5 → "12,5 %"), like delta_pct.
 * - `duration` values are seconds.
 */
export type ValueType =
  | "text"
  | "number"
  | "money"
  | "percent"
  | "date"
  | "datetime"
  | "duration"
  | "status"
  | "org"
  | "link";

export type NumericKind = "number" | "money" | "percent" | "duration";

/** How a number is shown (axis, tooltip, KPI, grid cell). */
export type ValueFormat = {
  kind?: NumericKind;
  /** ISO 4217 code for money (default: the formatter's money currency). */
  currency?: string;
  /** Suffix for plain numbers, already translated (e.g. "sa"). */
  suffix?: string;
  maximumFractionDigits?: number;
  /** Duration values: seconds per unit of the value (hours → 3600). */
  durationScale?: number;
};

export type DurationLabels = {
  day: string;
  hour: string;
  minute: string;
  second: string;
};

export const EMPTY = "—";

export function isNumber(v: unknown): v is number {
  return typeof v === "number" && Number.isFinite(v);
}

/** "1 g 3 sa", "2 sa 15 dk", "45 sn" — two largest units. */
export function formatDuration(
  seconds: number | null | undefined,
  fmt: Formatter,
  labels: DurationLabels,
): string {
  if (!isNumber(seconds)) return EMPTY;
  const sign = seconds < 0 ? "-" : "";
  let rest = Math.round(Math.abs(seconds));
  const parts: Array<[number, string]> = [];
  const units: Array<[number, string]> = [
    [86400, labels.day],
    [3600, labels.hour],
    [60, labels.minute],
    [1, labels.second],
  ];
  for (const [size, label] of units) {
    const n = Math.floor(rest / size);
    rest -= n * size;
    if (n > 0 || (parts.length === 0 && size === 1)) parts.push([n, label]);
    if (parts.length === 2) break;
  }
  return sign + parts.map(([n, l]) => `${fmt.number(n)} ${l}`).join(" ");
}

export function formatValue(
  value: number | null | undefined,
  format: ValueFormat | undefined,
  fmt: Formatter,
  durationLabels?: DurationLabels,
): string {
  if (!isNumber(value)) return EMPTY;
  const kind = format?.kind ?? "number";
  const digits = format?.maximumFractionDigits;
  switch (kind) {
    case "money":
      return format?.currency
        ? fmt.currency(value, format.currency)
        : fmt.money(value);
    case "percent":
      return fmt.number(value / 100, {
        style: "percent",
        maximumFractionDigits: digits ?? 1,
      });
    case "duration":
      return durationLabels
        ? formatDuration(
            value * (format?.durationScale ?? 1),
            fmt,
            durationLabels,
          )
        : fmt.number(value);
    default: {
      const text = fmt.number(value, { maximumFractionDigits: digits ?? 2 });
      return format?.suffix ? `${text} ${format.suffix}` : text;
    }
  }
}

/** Short axis tick: compact notation (1,2 B / 3,4 Mn) for big numbers. */
export function formatAxisValue(
  value: number,
  format: ValueFormat | undefined,
  fmt: Formatter,
  durationLabels?: DurationLabels,
): string {
  if (!isNumber(value)) return "";
  const kind = format?.kind ?? "number";
  if (kind === "percent") {
    return fmt.number(value / 100, {
      style: "percent",
      maximumFractionDigits: 0,
    });
  }
  if (kind === "duration") {
    const hours = (value * (format?.durationScale ?? 1)) / 3600;
    const text = fmt.number(hours, { maximumFractionDigits: 1 });
    return durationLabels ? `${text} ${durationLabels.hour}` : text;
  }
  const opts: Intl.NumberFormatOptions =
    Math.abs(value) >= 10000
      ? { notation: "compact", maximumFractionDigits: 1 }
      : { maximumFractionDigits: 1 };
  if (kind === "money") {
    return fmt.number(value, {
      ...opts,
      style: "currency",
      currency: format?.currency ?? "TRY",
      currencyDisplay: "narrowSymbol",
      maximumFractionDigits: opts.maximumFractionDigits ?? 0,
    });
  }
  return fmt.number(value, opts);
}

/** Signed percent for deltas: "+12,3 %", "−4 %", "0 %". */
export function formatDeltaPct(
  deltaPct: number | null | undefined,
  fmt: Formatter,
): string {
  if (!isNumber(deltaPct)) return EMPTY;
  return fmt.number(deltaPct / 100, {
    style: "percent",
    maximumFractionDigits: 1,
    signDisplay: "exceptZero",
  });
}

/** Percent change from previous to current; null when previous is 0/missing. */
export function deltaPct(
  current: number | null | undefined,
  previous: number | null | undefined,
): number | null {
  if (!isNumber(current) || !isNumber(previous) || previous === 0) return null;
  return ((current - previous) / Math.abs(previous)) * 100;
}

export type DeltaTone = "good" | "bad" | "neutral";

/** Good/bad reading of a delta given which direction is good. */
export function deltaTone(
  delta: number | null | undefined,
  good: "up" | "down" = "up",
): DeltaTone {
  if (!isNumber(delta) || delta === 0) return "neutral";
  const up = delta > 0;
  return up === (good === "up") ? "good" : "bad";
}

export type Granularity = "hour" | "day" | "week" | "month";

/** Localised axis / tooltip label for a time bucket key (ISO date). */
export function formatBucket(
  key: string,
  granularity: Granularity | undefined,
  fmt: Formatter,
  long = false,
): string {
  const date = new Date(key.length === 10 ? `${key}T00:00:00` : key);
  if (Number.isNaN(date.getTime())) return key;
  switch (granularity) {
    case "month":
      return fmt.dateParts(date, {
        month: long ? "long" : "short",
        year: "numeric",
      });
    case "hour":
      return fmt.dateParts(date, {
        hour: "2-digit",
        minute: "2-digit",
        ...(long ? { day: "numeric", month: "short" } : {}),
      });
    default:
      return fmt.dateParts(date, {
        day: "numeric",
        month: "short",
        ...(long ? { year: "numeric" } : {}),
      });
  }
}

/** Maps a series/KPI unit string of the envelope to a value format. */
export function unitFormat(unit: string | null | undefined): ValueFormat {
  const u = (unit ?? "").trim();
  if (!u || u === "count") return { kind: "number", maximumFractionDigits: 0 };
  if (u === "money") return { kind: "money" };
  if (/^[A-Z]{3}$/.test(u)) return { kind: "money", currency: u };
  if (u === "percent" || u === "%") return { kind: "percent" };
  if (u === "seconds" || u === "duration") return { kind: "duration" };
  if (u === "hours" || u === "hour")
    return { kind: "duration", durationScale: 3600 };
  if (u === "minutes" || u === "minute")
    return { kind: "duration", durationScale: 60 };
  return { kind: "number" };
}
