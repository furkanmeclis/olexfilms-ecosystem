/**
 * Localized numbers for charts: compact axis ticks, values by unit and the
 * bucket labels of the x axis. Takes the Intl locale tag (core/digits
 * `intlLocale`). Pure, so node --test runs it.
 */

/** Short axis tick: 1,2 B / 12 B / 1,5 Mn in the locale's own compact form. */
export function compact(v: number, locale: string): string {
  if (!Number.isFinite(v)) return '–';
  const abs = Math.abs(v);
  if (abs < 1000) return new Intl.NumberFormat(locale, { maximumFractionDigits: abs < 10 && abs % 1 ? 1 : 0 }).format(v);
  return new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 }).format(v);
}

/** Units the reports use (count, pieces, meters, money, percent as a 0..1 ratio, hours) with a fallback. */
export function formatValue(v: number | null | undefined, unit: string | null | undefined, locale: string, currency?: string | null): string {
  if (v === null || v === undefined || !Number.isFinite(v)) return '–';
  const nf = (o: Intl.NumberFormatOptions) => new Intl.NumberFormat(locale, o).format(v);
  switch (unit) {
    case 'money':
      if (currency) {
        try {
          return nf({ style: 'currency', currency, maximumFractionDigits: Math.abs(v) >= 1000 ? 0 : 2 });
        } catch {
          return `${nf({ maximumFractionDigits: 2 })} ${currency}`;
        }
      }
      return nf({ maximumFractionDigits: 2 });
    case 'percent':
      // Ratios 0..1 (TEC-1210 percent columns).
      return nf({ style: 'percent', maximumFractionDigits: 1 });
    case 'meters':
      return unitOr(() => nf({ style: 'unit', unit: 'meter', unitDisplay: 'short', maximumFractionDigits: 1 }), `${nf({ maximumFractionDigits: 1 })} m`);
    case 'hours':
      return unitOr(() => nf({ style: 'unit', unit: 'hour', unitDisplay: 'short', maximumFractionDigits: 1 }), `${nf({ maximumFractionDigits: 1 })} h`);
    default:
      return nf({ maximumFractionDigits: Math.abs(v) < 100 ? 1 : 0 });
  }
}

/** Intl unit style where the engine has it (Hermes may not), else a plain suffix. */
function unitOr(format: () => string, fallback: string): string {
  try {
    return format();
  } catch {
    return fallback;
  }
}

/** Parses a bucket key (YYYY-MM-DD or YYYY-MM) as a UTC date. */
export function bucketDate(key: string): Date | null {
  const m = /^(\d{4})-(\d{2})(?:-(\d{2}))?/.exec(key);
  if (!m) return null;
  return new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, m[3] ? Number(m[3]) : 1));
}

/**
 * x axis label of a bucket: day and week buckets as "12 Kas", month buckets
 * as "Kas 26"; `long` gives the tooltip form with the year.
 */
export function bucketLabel(key: string, granularity: string | null | undefined, locale: string, long = false): string {
  const d = bucketDate(key);
  if (!d) return key;
  const o: Intl.DateTimeFormatOptions =
    granularity === 'month'
      ? { month: long ? 'long' : 'short', year: long ? 'numeric' : '2-digit', timeZone: 'UTC' }
      : { day: 'numeric', month: 'short', ...(long ? { year: 'numeric' } : {}), timeZone: 'UTC' };
  return new Intl.DateTimeFormat(locale, o).format(d);
}

/** Granularity of a series from its bucket keys when the envelope does not say. */
export function guessGranularity(keys: readonly string[]): 'day' | 'week' | 'month' {
  if (keys.length < 2) return 'day';
  const a = bucketDate(keys[0]);
  const b = bucketDate(keys[1]);
  if (!a || !b) return 'day';
  const days = Math.round((b.getTime() - a.getTime()) / 86_400_000);
  return days >= 28 ? 'month' : days >= 7 ? 'week' : 'day';
}
