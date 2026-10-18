import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import { ApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { i18n } from '@/core/i18n';
import { PAGE_SIZE, usePagedQuery } from '@/features/admin/kit';

import { sharePanelPdf } from './pdf-share';

type S = components['schemas'];
export type EodReport = S['EodReport'];
export type EodGroup = S['EodGroupTotal']['group'];
export type EodTotals = S['EodTotals'];
export type WarehouseSettings = S['WarehouseSettings'];

/** Which reports the list shows: the system report, one warehouse's, or all. */
export type EodScope = 'all' | 'system' | 'warehouse';

export const eodKeys = {
  list: (scope: EodScope) => ['warehouse', 'eod', 'list', scope] as const,
  report: (uuid: string) => ['warehouse', 'eod', uuid] as const,
  settings: () => ['warehouse', 'settings'] as const,
};

export function useEodReports(scope: EodScope) {
  return usePagedQuery<EodReport>(eodKeys.list(scope), (offset) =>
    unwrap(
      api.GET('/v1/warehouse/eod-reports', {
        params: { query: { scope: scope === 'all' ? undefined : scope, limit: PAGE_SIZE, offset } },
      }),
    ),
  );
}

export function useEodReport(uuid: string) {
  return useQuery({
    queryKey: eodKeys.report(uuid),
    queryFn: () => unwrap(api.GET('/v1/warehouse/eod-reports/{uuid}', { params: { path: { uuid } } })),
  });
}

/** Builds (or rebuilds) the report of the open business day; without a warehouse it is the system report. */
export function generateEodReport(warehouseUuid?: string) {
  return unwrap(api.POST('/v1/warehouse/eod-reports', { body: warehouseUuid ? { warehouse_uuid: warehouseUuid } : {} }));
}

const POLL_MS = 1500;
const POLL_LIMIT = 40;

/**
 * Queues the PDF of a report, waits for the export job (worker-docs), then
 * downloads it and opens the share sheet.
 */
export async function shareEodPdf(report: EodReport) {
  let job = await unwrap(
    api.POST('/v1/warehouse/eod-reports/{uuid}/pdf', {
      params: { path: { uuid: report.uuid } },
      body: { locale: i18n.language },
    }),
  );
  for (let i = 0; i < POLL_LIMIT && (job.status === 'queued' || job.status === 'processing'); i++) {
    await new Promise((r) => setTimeout(r, POLL_MS));
    job = await unwrap(api.GET('/v1/warehouse/eod-report-pdfs/{uuid}', { params: { path: { uuid: job.uuid } } }));
  }
  if (job.status !== 'completed') {
    throw new ApiError(0, { code: job.status === 'failed' ? 'EXPORT_FAILED' : 'EXPORT_TIMEOUT', message: job.error ?? job.status });
  }
  const name = report.warehouse ? `${report.report_date}-${report.warehouse.code}` : report.report_date;
  await sharePanelPdf(`/v1/warehouse/eod-report-pdfs/${job.uuid}/download`, {}, `eod-${name}`);
}

export function useWarehouseSettings() {
  return useQuery({
    queryKey: eodKeys.settings(),
    queryFn: () => unwrap(api.GET('/v1/warehouse/settings')),
  });
}

export function saveWarehouseSettings(eodCutoff: string | null) {
  return unwrap(api.PUT('/v1/warehouse/settings', { body: { eod_cutoff: eodCutoff } }));
}
