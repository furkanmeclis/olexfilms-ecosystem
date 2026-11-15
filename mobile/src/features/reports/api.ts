import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import { api, unwrap } from '@/core/api/client';
import type { paths } from '@/core/api/schema';
import type { Envelope } from '@/features/analytics-kit/envelope';
import { type PeriodQuery, type PeriodState, type PeriodSupport, toQuery } from '@/features/analytics-kit/period';

type ReportOp = paths['/v1/reports/{report}']['get'];
type ApiReport = ReportOp['responses'][200]['content']['application/json']['data'];
/** The report envelope; v2 fields (TEC-1210) are read through the kit's `Envelope`. */
export type Report = ApiReport & Envelope;
export type ReportSeries = Report['series'][number];
export type CatalogItem =
  paths['/v1/reports/catalog']['get']['responses'][200]['content']['application/json']['data']['items'][number];
export type Period = '7d' | '30d' | '90d' | '12m';

export const PERIODS: Period[] = ['7d', '30d', '90d', '12m'];

/** Reports the caller may open (module on, permission, reach), localized by the API. */
export function useReportCatalog() {
  const { i18n } = useTranslation();
  return useQuery({
    queryKey: ['reports', 'catalog', i18n.language],
    queryFn: () => unwrap(api.GET('/v1/reports/catalog', { params: { query: { locale: i18n.language } } })),
  });
}

/**
 * What a catalog entry takes from the period bar: a period, a bucket size
 * (when it lists granularities) and a comparison (TEC-1210 `compare`, when
 * the catalog says so).
 */
export function supportOf(entry: CatalogItem | undefined): PeriodSupport {
  if (!entry) return { periodic: false };
  const e = entry as CatalogItem & { compare?: boolean; compares?: string[] };
  return {
    periodic: entry.periodic,
    granularity: entry.granularities.length > 0,
    compare: e.compare === true || (Array.isArray(e.compares) && e.compares.length > 0),
  };
}

type Extra = { limit?: number; all_time?: boolean };

/**
 * One report in the shared envelope. Parameters a report does not take are a
 * 400, so callers pass only what the catalog says the report takes: a legacy
 * `period`, or `range` (the period bar's query, compare included).
 */
export function useReport(
  key: string,
  opts: { period?: Period | 'today'; range?: PeriodQuery; limit?: number; allTime?: boolean; enabled?: boolean } = {},
) {
  const { i18n } = useTranslation();
  const { period, range, limit, allTime, enabled = true } = opts;
  const query: PeriodQuery & Extra = range ? { ...range } : period ? { period } : {};
  if (limit) query.limit = limit;
  if (allTime) query.all_time = true;
  return useQuery({
    queryKey: ['reports', 'report', key, query, i18n.language],
    enabled,
    queryFn: async () =>
      (await unwrap(
        api.GET('/v1/reports/{report}', {
          // `compare` is a TEC-1210 parameter; the cast keeps this compiling against an older schema.
          params: { path: { report: key }, query: { ...query, locale: i18n.language } as ReportOp['parameters']['query'] },
        }),
      )) as Report,
  });
}

/** A report driven by the period bar state; `support` drops what the report does not take. */
export function usePeriodReport(key: string, state: PeriodState, support: PeriodSupport, opts: { limit?: number; enabled?: boolean } = {}) {
  return useReport(key, { range: toQuery(state, support), limit: opts.limit, enabled: opts.enabled });
}


/**
 * A report by catalog key for the home screens: it runs only when the
 * catalog lists it for the caller (module, permission, reach), and sends only
 * the period parameters the report takes. `available` is false while the
 * catalog loads or when the report is not open to the caller.
 */
export function useCatalogReport(
  key: string,
  state: PeriodState,
  opts: { limit?: number; allTime?: boolean; enabled?: boolean; granularity?: boolean } = {},
) {
  const catalog = useReportCatalog();
  const entry = catalog.data?.items.find((i) => i.key === key);
  const support = supportOf(entry);
  if (opts.granularity === false) support.granularity = false;
  const allTime = !!opts.allTime && !!entry?.all_time;
  const query = useReport(key, {
    range: allTime ? undefined : toQuery(state, support),
    limit: entry?.limited ? opts.limit : undefined,
    allTime,
    enabled: !!entry && (opts.enabled ?? true),
  });
  return { query, entry, available: !!entry, loading: catalog.isPending || (!!entry && query.isPending) };
}
