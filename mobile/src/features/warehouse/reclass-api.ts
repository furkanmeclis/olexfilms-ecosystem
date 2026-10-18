import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { PAGE_SIZE, usePagedQuery } from '@/features/admin/kit';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type Reclassification = S['StockReclassification'];
export type ReclassStatus = S['StockReclassificationStatus'];
export type ReclassInput = S['StockReclassificationInput'];

export type ReclassFilter = 'pending' | 'decided' | 'all';

const FILTERS: Record<ReclassFilter, ReclassStatus[] | undefined> = {
  pending: ['pending'],
  decided: ['approved', 'rejected', 'cancelled'],
  all: undefined,
};

export const reclassKeys = {
  list: (filter: ReclassFilter, q: string) => ['warehouse', 'reclass', 'list', filter, q] as const,
  one: (uuid: string) => ['warehouse', 'reclass', uuid] as const,
};

export function reclassTone(s: ReclassStatus): StatusTone {
  if (s === 'approved') return 'done';
  if (s === 'rejected') return 'problem';
  if (s === 'pending') return 'progress';
  return 'neutral';
}

export function useReclassifications(filter: ReclassFilter, q: string) {
  return usePagedQuery<Reclassification>(reclassKeys.list(filter, q), (offset) =>
    unwrap(
      api.GET('/v1/stock/reclassifications', {
        params: { query: { status: FILTERS[filter], q: q || undefined, limit: PAGE_SIZE, offset } },
      }),
    ),
  );
}

export function useReclassification(uuid: string) {
  return useQuery({
    queryKey: reclassKeys.one(uuid),
    queryFn: () => unwrap(api.GET('/v1/stock/reclassifications/{uuid}', { params: { path: { uuid } } })),
  });
}

export function createReclassification(body: ReclassInput) {
  return unwrap(api.POST('/v1/stock/reclassifications', { body }));
}

export type ReclassDecision = 'approve' | 'reject' | 'cancel';

export function decideReclassification(uuid: string, decision: ReclassDecision, note?: string) {
  const params = { path: { uuid } };
  const body = note ? { note } : {};
  if (decision === 'approve') return unwrap(api.POST('/v1/stock/reclassifications/{uuid}/approve', { params, body }));
  if (decision === 'reject') return unwrap(api.POST('/v1/stock/reclassifications/{uuid}/reject', { params, body }));
  return unwrap(api.POST('/v1/stock/reclassifications/{uuid}/cancel', { params, body }));
}
