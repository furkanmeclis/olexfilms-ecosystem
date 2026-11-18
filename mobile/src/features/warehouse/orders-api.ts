import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api, unwrap, uploadForm } from '@/core/api/client';
import type { components } from '@/core/api/schema';

import { PAGE } from './api';
import { type OrderScope, orderScopeQuery } from './order-scopes';

type S = components['schemas'];
export type Order = S['Order'];
export type OrderItem = S['OrderItem'];
export type OrderStatus = S['OrderStatus'];
export type OrderRollSplit = S['OrderRollSplit'];
export type OrderSuggestions = S['OrderSuggestions'];
export type OrderPickList = S['OrderPickList'];
export type OrderPlacement = S['WarehouseOrderPlacement'];
export type DeliveryMethod = S['DeliveryMethod'];

export type { OrderScope } from './order-scopes';

export const orderKeys = {
  orders: (scope: OrderScope, warehouse = '', mine = false) => ['warehouse', 'orders', scope, warehouse, mine] as const,
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
export function useOrders(scope: OrderScope, enabled = true, warehouse?: { uuid: string | undefined; ready: boolean }, mine = false) {
  const wh = scope === 'prepare' ? (warehouse?.uuid ?? '') : '';
  // TEC-1228: "my jobs" — the orders the user took (seller side only).
  const picker = mine && scope === 'prepare' ? { picker: 'me' } : {};
  return useInfiniteQuery({
    queryKey: orderKeys.orders(scope, wh, mine && scope === 'prepare'),
    enabled: enabled && (warehouse?.ready ?? true),
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/orders', {
          params: { query: { ...orderScopeQuery(scope, wh || undefined), ...picker, limit: PAGE, offset: pageParam } },
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
export function usePendingOrderCount(enabled = true, warehouse?: string) {
  return useQuery({
    queryKey: ['warehouse', 'orders', 'pending-count', warehouse ?? ''],
    enabled,
    staleTime: 60_000,
    refetchInterval: 120_000,
    queryFn: async () => {
      // TEC-1224: the same filter as the warehouse home counters (orderScopeQuery).
      const page = await unwrap(api.GET('/v1/orders', { params: { query: { ...orderScopeQuery('prepare', warehouse), limit: 1, offset: 0 } } }));
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
            params: { query: { ...orderScopeQuery('prepare', warehouse.uuid, status), limit: 1, offset: 0 } },
          }),
        ).then((p) => p.total);
      const [open, fresh] = await Promise.all([count(orderScopeQuery('prepare').status ?? ''), count('approved')]);
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
  /** TEC-1228: take the warehouse job; takeover lets a manager take someone else's. */
  claim: (uuid: string, takeover = false) => unwrap(api.POST('/v1/orders/{uuid}/claim', { params: { path: { uuid } }, body: { takeover } })),
  release: (uuid: string) => unwrap(api.DELETE('/v1/orders/{uuid}/claim', { params: { path: { uuid } } })),
  /** TEC-1228: render the shipping note again (seller, after shipping). */
  regenerateShippingDocument: (uuid: string) => unwrap(api.POST('/v1/orders/{uuid}/shipping-document', { params: { path: { uuid } } })),
  /** TEC-1228: the buyer's signed delivery receipt photo (JPEG from the camera). */
  attachReceipt: (uuid: string, photoUri: string) => {
    const form = new FormData();
    // React Native's FormData takes a { uri, name, type } file part; uploadForm sends it with XMLHttpRequest.
    form.append('file', { uri: photoUri, name: 'receipt.jpg', type: 'image/jpeg' } as unknown as Blob);
    return uploadForm<Order>(`/v1/orders/${encodeURIComponent(uuid)}/receipt-document`, form);
  },
  place: (uuid: string, body: S['WarehouseOrderPlaceInput']) =>
    unwrap(api.POST('/v1/warehouse/orders/{uuid}/place', { params: { path: { uuid } }, body })),
};
