import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import { toApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { MOBILE_API_VERSION } from '@/core/config';

type S = components['schemas'];

export type Warehouse = S['Warehouse'];
export type WarehouseRoom = S['WarehouseRoom'];
export type ScanResult = S['WarehouseScanResult'];
export type ScanExpect = S['WarehouseScanInput']['expect'];
export type StockCount = S['StockCount'];
export type StockCountStatus = S['StockCountStatus'];
export type StockCountInput = S['StockCountInput'];
export type StockCountScan = S['StockCountScan'];
export type StockCountScanResult = S['StockCountScanResult'];
export type StockCountReport = S['StockCountReport'];
export type StockCountLine = S['StockCountLine'];
export type CountResolution = NonNullable<StockCountLine['resolution']>;
export type Transfer = S['WarehouseTransfer'];
export type TransferStatus = S['WarehouseTransferStatus'];
export type TransferLine = S['WarehouseTransferLine'];
export type StockEntry = S['StockEntry'];
export type StockEntryStatus = S['StockEntryStatus'];
export type StockEntryLine = S['StockEntryLine'];

/** `/v1/mobile/*` routes require the contract header in their typed params. */
export const mobile = { header: { 'X-Mobile-Api-Version': MOBILE_API_VERSION } } as const;

/** One page size for every warehouse list. */
export const PAGE = 30;

/** Calls that answer 204 (no envelope): only success or ApiError. */
export async function noContent(call: Promise<{ error?: unknown; response: Response }>) {
  const r = await call;
  if (r.error !== undefined || !r.response.ok) throw toApiError(r.response.status, r.error);
}

/** Query keys of the warehouse feature. */
export const wk = {
  all: ['warehouse'] as const,
  warehouses: () => ['warehouse', 'warehouses'] as const,
  rooms: (warehouse: string) => ['warehouse', 'rooms', warehouse] as const,
  transfers: (status: string) => ['warehouse', 'transfers', status] as const,
  transfer: (uuid: string) => ['warehouse', 'transfer', uuid] as const,
  counts: (status: string) => ['warehouse', 'counts', status] as const,
  count: (uuid: string) => ['warehouse', 'count', uuid] as const,
  countScans: (uuid: string) => ['warehouse', 'count', uuid, 'scans'] as const,
  countReport: (uuid: string) => ['warehouse', 'count', uuid, 'report'] as const,
  entries: (status: string) => ['warehouse', 'entries', status] as const,
  entry: (uuid: string) => ['warehouse', 'entry', uuid] as const,
  summary: () => ['warehouse', 'summary'] as const,
};

// --- warehouses ----------------------------------------------------------------

export function useWarehouses() {
  return useQuery({
    queryKey: wk.warehouses(),
    queryFn: () => unwrap(api.GET('/v1/mobile/warehouse/warehouses', { params: { ...mobile, query: { active: true } } })),
    select: (d) => d.items,
    staleTime: 5 * 60_000,
  });
}

/** Rooms of a warehouse (panel route through the mobile mirror). */
export function useRooms(warehouse: string | undefined) {
  return useQuery({
    queryKey: wk.rooms(warehouse ?? ''),
    enabled: !!warehouse,
    queryFn: () =>
      unwrap(api.GET('/v1/warehouse/warehouses/{uuid}/rooms', { params: { path: { uuid: warehouse ?? '' } } })),
    select: (d) => d.items.filter((r) => r.active),
  });
}

// --- scan ----------------------------------------------------------------------

export function resolveScan(code: string, opts: { expect?: ScanExpect; warehouse?: string } = {}) {
  return unwrap(
    api.POST('/v1/mobile/warehouse/scan', {
      params: mobile,
      body: { code, expect: opts.expect ?? 'any', warehouse_uuid: opts.warehouse },
    }),
  );
}

// --- transfers -----------------------------------------------------------------

export function useTransfers(status: string, enabled = true) {
  return useInfiniteQuery({
    queryKey: wk.transfers(status),
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/mobile/warehouse/transfers', {
          params: { ...mobile, query: { status: status || undefined, limit: PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
    placeholderData: keepPreviousData,
  });
}

export function useTransfer(uuid: string) {
  return useQuery({
    queryKey: wk.transfer(uuid),
    queryFn: () => unwrap(api.GET('/v1/mobile/warehouse/transfers/{uuid}', { params: { ...mobile, path: { uuid } } })),
  });
}

export const transferApi = {
  create: (body: S['WarehouseTransferInput']) =>
    unwrap(api.POST('/v1/mobile/warehouse/transfers', { params: mobile, body })),
  addLines: (uuid: string, body: S['WarehouseTransferLinesInput']) =>
    unwrap(api.POST('/v1/mobile/warehouse/transfers/{uuid}/lines', { params: { ...mobile, path: { uuid } }, body })),
  deleteLine: (uuid: string, line: string) =>
    unwrap(
      api.DELETE('/v1/mobile/warehouse/transfers/{uuid}/lines/{line_uuid}', {
        params: { ...mobile, path: { uuid, line_uuid: line } },
      }),
    ),
  place: (uuid: string, body: S['StockEntryPlaceInput']) =>
    unwrap(api.POST('/v1/mobile/warehouse/transfers/{uuid}/place', { params: { ...mobile, path: { uuid } }, body })),
  ship: (uuid: string) =>
    unwrap(api.POST('/v1/mobile/warehouse/transfers/{uuid}/ship', { params: { ...mobile, path: { uuid } } })),
  complete: (uuid: string, body?: S['WarehouseTransferCompleteInput']) =>
    unwrap(
      api.POST('/v1/mobile/warehouse/transfers/{uuid}/complete', { params: { ...mobile, path: { uuid } }, body: body ?? {} }),
    ),
  cancel: (uuid: string) =>
    unwrap(api.POST('/v1/mobile/warehouse/transfers/{uuid}/cancel', { params: { ...mobile, path: { uuid } } })),
};

// --- counts --------------------------------------------------------------------

export function useCounts(status: string, enabled = true) {
  return useInfiniteQuery({
    queryKey: wk.counts(status),
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/mobile/warehouse/stock-counts', {
          params: { ...mobile, query: { status: status || undefined, limit: PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
    placeholderData: keepPreviousData,
  });
}

export function useCount(uuid: string) {
  return useQuery({
    queryKey: wk.count(uuid),
    queryFn: () => unwrap(api.GET('/v1/mobile/warehouse/stock-counts/{uuid}', { params: { ...mobile, path: { uuid } } })),
  });
}

export function useCountScans(uuid: string, enabled: boolean) {
  return useQuery({
    queryKey: wk.countScans(uuid),
    enabled,
    queryFn: () =>
      unwrap(
        api.GET('/v1/mobile/warehouse/stock-counts/{uuid}/scans', {
          params: { ...mobile, path: { uuid }, query: { limit: 200 } },
        }),
      ),
  });
}

export function useCountReport(uuid: string, enabled: boolean) {
  return useQuery({
    queryKey: wk.countReport(uuid),
    enabled,
    queryFn: () =>
      unwrap(api.GET('/v1/mobile/warehouse/stock-counts/{uuid}/report', { params: { ...mobile, path: { uuid } } })),
  });
}

export const countApi = {
  create: (body: StockCountInput) => unwrap(api.POST('/v1/mobile/warehouse/stock-counts', { params: mobile, body })),
  approveStart: (uuid: string) =>
    unwrap(api.POST('/v1/mobile/warehouse/stock-counts/{uuid}/approve-start', { params: { ...mobile, path: { uuid } } })),
  start: (uuid: string) =>
    unwrap(api.POST('/v1/mobile/warehouse/stock-counts/{uuid}/start', { params: { ...mobile, path: { uuid } } })),
  scan: (uuid: string, body: S['StockCountScanInput']) =>
    unwrap(api.POST('/v1/mobile/warehouse/stock-counts/{uuid}/scans', { params: { ...mobile, path: { uuid } }, body })),
  deleteScan: (uuid: string, scan: string) =>
    noContent(
      api.DELETE('/v1/mobile/warehouse/stock-counts/{uuid}/scans/{scan_uuid}', {
        params: { ...mobile, path: { uuid, scan_uuid: scan } },
      }),
    ),
  complete: (uuid: string) =>
    unwrap(api.POST('/v1/mobile/warehouse/stock-counts/{uuid}/complete', { params: { ...mobile, path: { uuid } } })),
  approve: (uuid: string, body: S['StockCountApproveInput']) =>
    unwrap(api.POST('/v1/mobile/warehouse/stock-counts/{uuid}/approve', { params: { ...mobile, path: { uuid } }, body })),
  cancel: (uuid: string) =>
    unwrap(api.POST('/v1/mobile/warehouse/stock-counts/{uuid}/cancel', { params: { ...mobile, path: { uuid } } })),
};

// --- stock entries (panel routes through the mobile mirror) -----------------------

export function useEntries(status: string, enabled = true) {
  return useInfiniteQuery({
    queryKey: wk.entries(status),
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/warehouse/stock-entries', {
          params: { query: { status: status || undefined, limit: PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
    placeholderData: keepPreviousData,
  });
}

export function useEntry(uuid: string) {
  return useQuery({
    queryKey: wk.entry(uuid),
    queryFn: () => unwrap(api.GET('/v1/warehouse/stock-entries/{uuid}', { params: { path: { uuid } } })),
  });
}

export const entryApi = {
  create: (body: S['StockEntryInput']) => unwrap(api.POST('/v1/warehouse/stock-entries', { body })),
  addLines: (uuid: string, body: S['StockEntryLinesInput']) =>
    unwrap(api.POST('/v1/warehouse/stock-entries/{uuid}/lines', { params: { path: { uuid } }, body })),
  deleteLine: (uuid: string, line: string) =>
    unwrap(
      api.DELETE('/v1/warehouse/stock-entries/{uuid}/lines/{line_uuid}', { params: { path: { uuid, line_uuid: line } } }),
    ),
  place: (uuid: string, body: S['StockEntryPlaceInput']) =>
    unwrap(api.POST('/v1/warehouse/stock-entries/{uuid}/place', { params: { path: { uuid } }, body })),
  confirm: (uuid: string) => unwrap(api.POST('/v1/warehouse/stock-entries/{uuid}/confirm', { params: { path: { uuid } } })),
  cancel: (uuid: string) => unwrap(api.POST('/v1/warehouse/stock-entries/{uuid}/cancel', { params: { path: { uuid } } })),
};

// --- home summary ---------------------------------------------------------------

/**
 * Open work for the home counters. No summary endpoint exists yet, so this is
 * three list calls with limit 1 (only `total` is read).
 */
export function useWarehouseSummary(canEntries: boolean) {
  return useQuery({
    queryKey: wk.summary(),
    queryFn: async () => {
      const [transfers, counts, entries] = await Promise.all([
        unwrap(
          api.GET('/v1/mobile/warehouse/transfers', {
            params: { ...mobile, query: { status: 'draft,in_transit', limit: 1 } },
          }),
        ),
        unwrap(
          api.GET('/v1/mobile/warehouse/stock-counts', {
            params: { ...mobile, query: { status: 'draft,in_progress,pending_review', limit: 1 } },
          }),
        ),
        canEntries
          ? unwrap(api.GET('/v1/warehouse/stock-entries', { params: { query: { status: 'draft', limit: 1 } } })).catch(
              () => null,
            )
          : Promise.resolve(null),
      ]);
      return { transfers: transfers.total, counts: counts.total, entries: entries?.total ?? null };
    },
  });
}

// --- bin to bin move (panel route through the mobile mirror) ---------------------

export function moveUnits(body: S['WarehouseMoveInput']) {
  return unwrap(api.POST('/v1/warehouse/moves', { body }));
}
