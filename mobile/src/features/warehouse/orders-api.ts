import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { sellerWarehouseFilter } from '@/features/orders/order-summary';

import { PAGE } from './api';

type S = components['schemas'];
export type Order = S['Order'];
export type OrderItem = S['OrderItem'];
export type OrderStatus = S['OrderStatus'];
export type OrderRollSplit = S['OrderRollSplit'];
export type OrderSuggestions = S['OrderSuggestions'];
export type OrderPickList = S['OrderPickList'];
export type OrderPlacement = S['WarehouseOrderPlacement'];
export type DeliveryMethod = S['DeliveryMethod'];

/** Warehouse view of orders: what to prepare (seller) and what to receive and shelve (buyer). */
export type OrderScope = 'prepare' | 'receive' | 'all';

const SCOPES: Record<OrderScope, { side?: 'seller' | 'buyer'; status?: string }> = {
  prepare: { side: 'seller', status: 'approved,processing,preparing,ready' },
  receive: { side: 'buyer', status: 'shipped,received' },
  all: {},
};

export const orderKeys = {
  orders: (scope: OrderScope, warehouse = '') => ['warehouse', 'orders', scope, warehouse] as const,
  counts: (warehouse = '') => ['warehouse', 'orders', 'counts', warehouse] as const,
  order: (uuid: string) => ['warehouse', 'order', uuid] as const,
  pickList: (uuid: string) => ['warehouse', 'order', uuid, 'pick-list'] as const,
  suggestions: (uuid: string, item: string, unlocated: boolean) =>
    ['warehouse', 'order', uuid, 'suggest', item, unlocated] as const,
  pickLocations: (uuid: string, item: string) => ['warehouse', 'order', uuid, 'locations', item] as const,
  unplaced: (uuid: string) => ['warehouse', 'order', uuid, 'unplaced'] as const,
  deliveryMethods: () => ['warehouse', 'delivery-methods'] as const,
};

/**
 * Orders of one scope. TEC-1067 / TEC-1049: the orders to prepare are the
 * picked warehouse's (`seller_warehouse`); the receive side has no warehouse
 * filter in the API, so it lists the organization's.
 */
export function useOrders(scope: OrderScope, enabled = true, warehouse?: { uuid: string | undefined; ready: boolean }) {
  const wh = scope === 'prepare' ? (warehouse?.uuid ?? '') : '';
  return useInfiniteQuery({
    queryKey: orderKeys.orders(scope, wh),
    enabled: enabled && (warehouse?.ready ?? true),
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/orders', {
          params: { query: { ...SCOPES[scope], ...sellerWarehouseFilter(wh || undefined), limit: PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
    // Placeholder rows only from the same warehouse: a switch must not keep the old one's orders.
    placeholderData: (prev, prevQuery) => (prevQuery?.queryKey[3] === wh ? prev : undefined),
  });
}

/**
 * Orders waiting for this organization's warehouse (seller side, approved up to
 * ready): the count shown on the warehouse and center home shortcuts (TEC-1025).
 */
export function usePendingOrderCount(enabled = true) {
  return useQuery({
    queryKey: ['warehouse', 'orders', 'pending-count'],
    enabled,
    staleTime: 60_000,
    refetchInterval: 120_000,
    queryFn: async () => {
      const page = await unwrap(api.GET('/v1/orders', { params: { query: { ...SCOPES.prepare, limit: 1, offset: 0 } } }));
      return page.total;
    },
  });
}

/**
 * Warehouse home counters (TEC-1067, revize 7.2) of the picked warehouse:
 * open = every order still to ship (approved up to ready), new = approved and
 * not started yet.
 */
export function useWarehouseOrderCounts(enabled: boolean, warehouse: { uuid: string | undefined; ready: boolean }) {
  return useQuery({
    queryKey: orderKeys.counts(warehouse.uuid),
    enabled: enabled && warehouse.ready,
    staleTime: 30_000,
    refetchInterval: 120_000,
    queryFn: async () => {
      const count = (status: string) =>
        unwrap(
          api.GET('/v1/orders', {
            params: { query: { side: 'seller', status, ...sellerWarehouseFilter(warehouse.uuid), limit: 1, offset: 0 } },
          }),
        ).then((p) => p.total);
      const [open, fresh] = await Promise.all([count(SCOPES.prepare.status ?? ''), count('approved')]);
      return { open, new: fresh };
    },
  });
}

export function useOrder(uuid: string) {
  return useQuery({
    queryKey: orderKeys.order(uuid),
    queryFn: () => unwrap(api.GET('/v1/orders/{uuid}', { params: { path: { uuid } } })),
  });
}

export function usePickList(uuid: string, enabled: boolean) {
  return useQuery({
    queryKey: orderKeys.pickList(uuid),
    enabled,
    queryFn: () => unwrap(api.GET('/v1/orders/{uuid}/pick-list', { params: { path: { uuid } } })),
  });
}

export function usePickLocations(uuid: string, item: string | null) {
  return useQuery({
    queryKey: orderKeys.pickLocations(uuid, item ?? ''),
    enabled: !!item,
    queryFn: () =>
      unwrap(
        api.GET('/v1/orders/{uuid}/items/{item_uuid}/pick-locations', {
          params: { path: { uuid, item_uuid: item ?? '' } },
        }),
      ),
    select: (d) => d.items,
  });
}

export function useSuggestions(uuid: string, item: string | null, unlocated: boolean) {
  return useQuery({
    queryKey: orderKeys.suggestions(uuid, item ?? '', unlocated),
    enabled: !!item,
    staleTime: 0,
    queryFn: () =>
      unwrap(
        api.GET('/v1/orders/{uuid}/items/{item_uuid}/suggestions', {
          params: { path: { uuid, item_uuid: item ?? '' }, query: { include_unlocated: unlocated } },
        }),
      ),
  });
}

/** Units of a received order still waiting for a shelf (buyer side). */
export function useUnplaced(uuid: string, enabled: boolean) {
  return useQuery({
    queryKey: orderKeys.unplaced(uuid),
    enabled,
    queryFn: () => unwrap(api.GET('/v1/warehouse/orders/{uuid}/unplaced', { params: { path: { uuid } } })),
  });
}

/**
 * The seller's delivery methods for shipping. Optional: when the list cannot
 * be read the order ships without a method.
 */
export function useDeliveryMethods(enabled: boolean) {
  return useQuery({
    queryKey: orderKeys.deliveryMethods(),
    enabled,
    retry: false,
    staleTime: 10 * 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/delivery-methods', { params: { query: { active: true, limit: 100 } } })),
    select: (d) => d.items,
  });
}

export const orderApi = {
  transition: (uuid: string, body: S['OrderTransitionInput']) =>
    unwrap(api.POST('/v1/orders/{uuid}/transitions', { params: { path: { uuid } }, body })),
  assign: (uuid: string, item: string, body: S['OrderUnitAssignInput']) =>
    unwrap(api.POST('/v1/orders/{uuid}/items/{item_uuid}/units', { params: { path: { uuid, item_uuid: item } }, body })),
  unassign: (uuid: string, item: string, unit: string) =>
    unwrap(
      api.DELETE('/v1/orders/{uuid}/items/{item_uuid}/units/{unit_uuid}', {
        params: { path: { uuid, item_uuid: item, unit_uuid: unit } },
      }),
    ),
  suggestions: (uuid: string, item: string, unlocated: boolean) =>
    unwrap(
      api.GET('/v1/orders/{uuid}/items/{item_uuid}/suggestions', {
        params: { path: { uuid, item_uuid: item }, query: { include_unlocated: unlocated } },
      }),
    ),
  applySuggestions: (uuid: string, item: string, body: S['OrderSuggestionsApplyInput']) =>
    unwrap(
      api.POST('/v1/orders/{uuid}/items/{item_uuid}/suggestions/apply', {
        params: { path: { uuid, item_uuid: item } },
        body,
      }),
    ),
  place: (uuid: string, body: S['WarehouseOrderPlaceInput']) =>
    unwrap(api.POST('/v1/warehouse/orders/{uuid}/place', { params: { path: { uuid } }, body })),
};
