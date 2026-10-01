/**
 * Locale aware formatting with Intl (TEC-137). Dates arrive from the API in
 * UTC and are shown in the viewer's effective time zone
 * (Me.effective_timezone: user -> organization -> brand center ->
 * Europe/Istanbul); numbers and money follow the effective locale and the
 * organization's currency.
 *
 * Components use `useLocale().format`, which binds locale and time zone;
 * these plain functions are for code outside React and for tests.
 */
import { i18nConfig, type AppLocale } from "@/config/i18n";

export type FormatContext = {
  locale: AppLocale;
  /** IANA name, e.g. "Asia/Dubai". Defaults to Europe/Istanbul. */
  timeZone?: string;
};

export type DateInput = Date | string | number | null | undefined;

/** Shown for a missing or unparsable date. */
export const EMPTY_VALUE = "—";

export type DateStyle = "short" | "medium" | "long";

const cache = new Map<string, Intl.DateTimeFormat | Intl.NumberFormat>();

function cached<T extends Intl.DateTimeFormat | Intl.NumberFormat>(
  kind: string,
  locale: string,
  options: object,
  make: () => T,
): T {
  const key = `${kind}|${locale}|${JSON.stringify(options)}`;
  let hit = cache.get(key) as T | undefined;
  if (!hit) {
    hit = make();
    cache.set(key, hit);
  }
  return hit;
}

/** True when Intl knows the IANA zone name. */
export function isValidTimeZone(timeZone: string | null | undefined): boolean {
  if (!timeZone) return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone });
    return true;
  } catch {
    return false;
  }
}

function zoneOf(ctx: FormatContext): string {
  return ctx.timeZone && isValidTimeZone(ctx.timeZone)
    ? ctx.timeZone
    : i18nConfig.defaultTimeZone;
}

export function toDate(value: DateInput): Date | null {
  if (value === null || value === undefined || value === "") return null;
  const d = value instanceof Date ? value : new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

function dateTimeFormat(
  ctx: FormatContext,
  options: Intl.DateTimeFormatOptions,
) {
  const opts = { ...options, timeZone: zoneOf(ctx) };
  return cached(
    "dt",
    ctx.locale,
    opts,
    () => new Intl.DateTimeFormat(ctx.locale, opts),
  );
}

const DATE_OPTIONS: Record<DateStyle, Intl.DateTimeFormatOptions> = {
  short: { year: "numeric", month: "2-digit", day: "2-digit" },
  medium: { year: "numeric", month: "short", day: "numeric" },
  long: { year: "numeric", month: "long", day: "numeric", weekday: "long" },
};

/** Formats with arbitrary Intl options in the context's time zone. */
export function formatDateParts(
  value: DateInput,
  ctx: FormatContext,
  options: Intl.DateTimeFormatOptions,
): string {
  const d = toDate(value);
  return d ? dateTimeFormat(ctx, options).format(d) : EMPTY_VALUE;
}

/** Calendar date in the viewer's zone: 01.01.2026 (tr), 01/01/2026 (en). */
export function formatDate(
  value: DateInput,
  ctx: FormatContext,
  style: DateStyle = "short",
): string {
  return formatDateParts(value, ctx, DATE_OPTIONS[style]);
}

/** Wall-clock time in the viewer's zone: 16:00 (tr, Asia/Dubai). */
export function formatTime(
  value: DateInput,
  ctx: FormatContext,
  { seconds = false }: { seconds?: boolean } = {},
): string {
  return formatDateParts(value, ctx, {
    hour: "2-digit",
    minute: "2-digit",
    ...(seconds ? { second: "2-digit" } : {}),
  });
}

/** Date and time in the viewer's zone: 01.01.2026 16:00. */
export function formatDateTime(
  value: DateInput,
  ctx: FormatContext,
  {
    style = "short",
    seconds = false,
    timeZoneName = false,
  }: { style?: DateStyle; seconds?: boolean; timeZoneName?: boolean } = {},
): string {
  return formatDateParts(value, ctx, {
    ...DATE_OPTIONS[style],
    hour: "2-digit",
    minute: "2-digit",
    ...(seconds ? { second: "2-digit" } : {}),
    ...(timeZoneName ? { timeZoneName: "short" } : {}),
  });
}

const RELATIVE_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
  ["second", 1],
];

const relativeCache = new Map<string, Intl.RelativeTimeFormat>();

/** "3 hours ago" / "3 saat önce" / "in 2 days". */
export function formatRelative(
  value: DateInput,
  ctx: FormatContext,
  now: DateInput = Date.now(),
): string {
  const d = toDate(value);
  const base = toDate(now);
  if (!d || !base) return EMPTY_VALUE;
  const diff = (d.getTime() - base.getTime()) / 1000;
  let rtf = relativeCache.get(ctx.locale);
  if (!rtf) {
    rtf = new Intl.RelativeTimeFormat(ctx.locale, { numeric: "auto" });
    relativeCache.set(ctx.locale, rtf);
  }
  for (const [unit, seconds] of RELATIVE_UNITS) {
    if (Math.abs(diff) >= seconds || unit === "second") {
      return rtf.format(Math.round(diff / seconds), unit);
    }
  }
  return EMPTY_VALUE;
}

function numberFormat(ctx: FormatContext, options: Intl.NumberFormatOptions) {
  return cached(
    "nf",
    ctx.locale,
    options,
    () => new Intl.NumberFormat(ctx.locale, options),
  );
}

export function formatNumber(
  value: number | null | undefined,
  ctx: FormatContext,
  options: Intl.NumberFormatOptions = {},
): string {
  if (value === null || value === undefined || Number.isNaN(value))
    return EMPTY_VALUE;
  return numberFormat(ctx, options).format(value);
}

/** Amount with 2 decimals and no symbol (ledger columns). */
export function formatMoney(
  value: number | null | undefined,
  ctx: FormatContext,
): string {
  return formatNumber(value, ctx, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

/**
 * Amount in an ISO 4217 currency (K7: organization currency, CHAR(3)).
 * An invalid code falls back to the plain amount plus the code.
 */
export function formatCurrency(
  value: number | null | undefined,
  currency: string,
  ctx: FormatContext,
): string {
  if (value === null || value === undefined || Number.isNaN(value))
    return EMPTY_VALUE;
  const code = currency.trim().toUpperCase();
  try {
    return numberFormat(ctx, { style: "currency", currency: code }).format(
      value,
    );
  } catch {
    return `${formatMoney(value, ctx)} ${code}`;
  }
}

export function formatPercent(
  value: number | null | undefined,
  ctx: FormatContext,
  maximumFractionDigits = 0,
): string {
  return formatNumber(value, ctx, { style: "percent", maximumFractionDigits });
}

/** IANA zones Intl knows, for the time zone picker. */
export function supportedTimeZones(): string[] {
  const intl = Intl as typeof Intl & {
    supportedValuesOf?: (key: "timeZone") => string[];
  };
  const zones = intl.supportedValuesOf?.("timeZone") ?? [];
  // Some engines omit UTC from the list; the backend accepts it.
  return zones.includes("UTC") ? zones : ["UTC", ...zones];
}

/** "GMT+04:00"-style offset of a zone right now, for picker labels. */
export function timeZoneOffsetLabel(
  timeZone: string,
  at: Date = new Date(),
): string {
  try {
    const part = new Intl.DateTimeFormat("en-US", {
      timeZone,
      timeZoneName: "longOffset",
    })
      .formatToParts(at)
      .find((p) => p.type === "timeZoneName");
    return part?.value ?? "";
  } catch {
    return "";
  }
}

export type Formatter = {
  locale: AppLocale;
  timeZone: string;
  date: (value: DateInput, style?: DateStyle) => string;
  time: (value: DateInput, options?: { seconds?: boolean }) => string;
  dateTime: (
    value: DateInput,
    options?: { style?: DateStyle; seconds?: boolean; timeZoneName?: boolean },
  ) => string;
  dateParts: (value: DateInput, options: Intl.DateTimeFormatOptions) => string;
  relative: (value: DateInput, now?: DateInput) => string;
  number: (
    value: number | null | undefined,
    options?: Intl.NumberFormatOptions,
  ) => string;
  money: (value: number | null | undefined) => string;
  currency: (value: number | null | undefined, currency: string) => string;
  percent: (
    value: number | null | undefined,
    maximumFractionDigits?: number,
  ) => string;
};

/** Binds locale and time zone once (LocaleProvider exposes it). */
export function createFormatter(ctx: FormatContext): Formatter {
  const c: FormatContext = { locale: ctx.locale, timeZone: zoneOf(ctx) };
  return {
    locale: c.locale,
    timeZone: c.timeZone as string,
    date: (v, style) => formatDate(v, c, style),
    time: (v, o) => formatTime(v, c, o),
    dateTime: (v, o) => formatDateTime(v, c, o),
    dateParts: (v, o) => formatDateParts(v, c, o),
    relative: (v, now) => formatRelative(v, c, now),
    number: (v, o) => formatNumber(v, c, o),
    money: (v) => formatMoney(v, c),
    currency: (v, code) => formatCurrency(v, code, c),
    percent: (v, digits) => formatPercent(v, c, digits),
  };
}
