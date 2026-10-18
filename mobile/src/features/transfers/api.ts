import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { PAGE_SIZE, usePagedQuery } from '@/features/admin/kit';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type Transfer = S['StockTransfer'];
export type TransferStatus = S['StockTransferStatus'];
export type TransferKind = S['StockTransferKind'];
export type TransferItemInput = S['StockTransferItemInput'];
export type OrgRef = S['OrderOrgRef'];

export type TransferFilter = 'all' | 'open' | 'done' | 'closed';

const FILTERS: Record<TransferFilter, TransferStatus[] | undefined> = {
  all: undefined,
  open: ['requested', 'approved', 'shipped'],
  done: ['received'],
  closed: ['rejected', 'cancelled'],
};

export const transferKeys = {
  list: (filter: TransferFilter, q: string) => ['transfers', 'list', filter, q] as const,
  one: (uuid: string) => ['transfers', uuid] as const,
  targets: (kind: TransferKind) => ['transfers', 'targets', kind] as const,
};

export function transferTone(s: TransferStatus): StatusTone {
  if (s === 'received') return 'done';
  if (s === 'rejected' || s === 'cancelled') return 'problem';
  return 'progress';
}

/** A rejection or cancellation carries a reason (web transfer-reason dialog). */
export const NEEDS_REASON: TransferStatus[] = ['rejected', 'cancelled'];

export function useTransfers(filter: TransferFilter, q: string) {
  return usePagedQuery<Transfer>(transferKeys.list(filter, q), (offset) =>
    unwrap(
      api.GET('/v1/stock-transfers', {
        params: { query: { status: FILTERS[filter]?.join(','), q: q || undefined, limit: PAGE_SIZE, offset } },
      }),
    ),
  );
}

export function useTransfer(uuid: string) {
  return useQuery({
    queryKey: transferKeys.one(uuid),
    queryFn: () => unwrap(api.GET('/v1/stock-transfers/{uuid}', { params: { path: { uuid } } })),
  });
}

/** Sibling dealers (same parent, K13) or, for a return, the direct parent only. */
export function useTransferTargets(kind: TransferKind) {
  return useQuery({
    queryKey: transferKeys.targets(kind),
    queryFn: () => unwrap(api.GET('/v1/stock-transfers/targets', { params: { query: { kind } } })),
    select: (d) => d.items,
  });
}

export function createTransfer(body: S['StockTransferCreateInput']) {
  return unwrap(api.POST('/v1/stock-transfers', { body }));
}

export function moveTransfer(uuid: string, status: TransferStatus, reason?: string) {
  return unwrap(
    api.POST('/v1/stock-transfers/{uuid}/transitions', {
      params: { path: { uuid } },
      body: reason ? { status, reason } : { status },
    }),
  );
}
