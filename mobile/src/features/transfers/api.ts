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
  returnSources: ['transfers', 'return-sources'] as const,
  returnOrders: (fromOrg: string) => ['transfers', 'return-orders', fromOrg] as const,
  returnOrderUnits: (order: string, fromOrg: string) => ['transfers', 'return-order-units', order, fromOrg] as const,
  // Under 'list' so a new return or a transition refreshes the order page too.
  orderReturns: (order: string) => ['transfers', 'list', 'order', order] as const,
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

/**
 * Sibling dealers (same parent, K13) or, for a return, the direct parent only.
 * The route needs transfers.request: a parent with only transfers.approve skips it.
 */
export function useTransferTargets(kind: TransferKind, enabled = true) {
  return useQuery({
    queryKey: transferKeys.targets(kind),
    queryFn: () => unwrap(api.GET('/v1/stock-transfers/targets', { params: { query: { kind } } })),
    select: (d) => d.items,
    enabled,
  });
}

/** TEC-1085: the direct dealer / distributor children a parent (transfers.approve) opens a return for. */
export function useReturnSources(enabled: boolean) {
  return useQuery({
    queryKey: transferKeys.returnSources,
    queryFn: () => unwrap(api.GET('/v1/stock-transfers/return-sources')),
    select: (d) => d.items,
    enabled,
  });
}

/**
 * TEC-1085: orders a return may refer to (sold by the parent to the giver, no
 * draft or cancelled one). fromOrg is the child of a return the parent opens,
 * '' for the active organization's own return.
 */
export function useReturnOrders(fromOrg: string, enabled: boolean) {
  return useQuery({
    queryKey: transferKeys.returnOrders(fromOrg),
    queryFn: () =>
      unwrap(api.GET('/v1/stock-transfers/return-orders', { params: { query: fromOrg ? { from_org_uuid: fromOrg } : {} } })),
    select: (d) => d.items,
    enabled,
  });
}

/** TEC-1085: the units of the order offered for the return, with what the giver may still return. */
export function useReturnOrderUnits(order: string, fromOrg: string, enabled: boolean) {
  return useQuery({
    queryKey: transferKeys.returnOrderUnits(order, fromOrg),
    queryFn: () =>
      unwrap(
        api.GET('/v1/stock-transfers/return-orders/{uuid}/units', {
          params: { path: { uuid: order }, query: fromOrg ? { from_org_uuid: fromOrg } : {} },
        }),
      ),
    select: (d) => d.items,
    enabled: enabled && order !== '',
  });
}

/** TEC-1085: the returns linked to an order (web OrderReturnsCard: kind=return, order_uuid, first 50). */
export function useOrderReturns(order: string, enabled: boolean) {
  return useQuery({
    queryKey: transferKeys.orderReturns(order),
    queryFn: () =>
      unwrap(api.GET('/v1/stock-transfers', { params: { query: { kind: 'return', order_uuid: order, limit: 50, offset: 0 } } })),
    select: (d) => d.items,
    enabled,
    retry: false,
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
