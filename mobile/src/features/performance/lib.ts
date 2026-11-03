/** Monthly metric keys of the performance module (backend model.Metrics), same as the web. */
export const METRICS = [
  'services_count',
  'order_volume',
  'warranty_start_rate',
  'measurement_rate',
  'certificate_coverage',
  'lead_conversion_rate',
  'review_avg',
  'stock_turnover',
  'waste_ratio',
  'cari_overdue_amount',
  'cari_overdue_days',
  'contract_days_left',
] as const;

export type Metric = (typeof METRICS)[number];
export type MetricKind = 'count' | 'ratio' | 'money' | 'days' | 'score';

const KIND: Record<Metric, MetricKind> = {
  services_count: 'count',
  order_volume: 'money',
  warranty_start_rate: 'ratio',
  measurement_rate: 'ratio',
  certificate_coverage: 'ratio',
  lead_conversion_rate: 'ratio',
  review_avg: 'score',
  stock_turnover: 'score',
  waste_ratio: 'ratio',
  cari_overdue_amount: 'money',
  cari_overdue_days: 'days',
  contract_days_left: 'days',
};

/** Metrics shown on a ranking row, in this order (the first with a value win). */
export const ROW_METRICS: Metric[] = ['services_count', 'order_volume', 'warranty_start_rate', 'measurement_rate'];

export function metricKind(metric: string): MetricKind {
  return KIND[metric as Metric] ?? 'score';
}

export type MetricValue = { value?: string; currency?: string | null } | undefined | null;

export function metricNumber(v: MetricValue): number | null {
  if (!v || v.value === undefined || v.value === '') return null;
  const n = Number(v.value);
  return Number.isFinite(n) ? n : null;
}

type Formatters = { number: (n: number, digits?: number) => string; money: (n: number, currency: string) => string };

/**
 * Display text of a metric value. Ratios arrive as 0..1 fractions on the web
 * (shown as percent); a missing value is a dash.
 */
export function formatMetric(metric: string, v: MetricValue, fmt: Formatters, fallbackCurrency?: string | null): string {
  const n = metricNumber(v);
  if (n === null) return '–';
  switch (metricKind(metric)) {
    case 'ratio':
      return `${fmt.number(n * 100, 1)}%`;
    case 'money': {
      const cur = v?.currency ?? fallbackCurrency;
      return cur ? fmt.money(n, cur) : fmt.number(n, 2);
    }
    case 'count':
    case 'days':
      return fmt.number(n, 0);
    default:
      return fmt.number(n, 2);
  }
}

/**
 * Chip label of a `YYYY-MM` period: `thisMonth` for the first (current)
 * month, else the local month name and year ("Kasım 2026"). `locale` is an
 * Intl locale (digits already chosen by the caller).
 */
export function monthLabel(period: string, locale: string, thisMonth: string | null): string {
  if (thisMonth) return thisMonth;
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  if (!m) return period;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, 1);
  return new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric' }).format(d);
}

/** The last `count` months (YYYY-MM), newest first. */
export function recentMonths(now: Date, count = 6): string[] {
  const out: string[] = [];
  for (let i = 0; i < count; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    out.push(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`);
  }
  return out;
}
