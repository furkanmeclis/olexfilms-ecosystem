import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import { api, unwrap } from '@/core/api/client';
import type { paths } from '@/core/api/schema';

type ReportOp = paths['/v1/reports/{report}']['get'];
export type Report = ReportOp['responses'][200]['content']['application/json']['data'];
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
 * One report in the shared envelope. Parameters a report does not take are a
 * 400, so period / limit are sent only when the catalog says the report takes them.
 */
export function useReport(
  key: string,
  opts: { period?: Period; limit?: number; enabled?: boolean } = {},
) {
  const { i18n } = useTranslation();
  const { period, limit, enabled = true } = opts;
  return useQuery({
    queryKey: ['reports', 'report', key, period ?? null, limit ?? null, i18n.language],
    enabled,
    queryFn: () =>
      unwrap(
        api.GET('/v1/reports/{report}', {
          params: { path: { report: key }, query: { period, limit, locale: i18n.language } },
        }),
      ),
  });
}
