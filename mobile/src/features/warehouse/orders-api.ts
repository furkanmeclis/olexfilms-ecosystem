import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

import { PAGE } from './api';

type S = components['schemas'];
export type Order = S['Order'];
export type OrderItem = S['OrderItem'];
export type OrderStatus = S['OrderStatus'];
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
  orders: (scope: OrderScope) => ['warehouse', 'orders', scope] as const,
  order: (uuid: string) => ['warehouse', 'order', uuid] as const,
  pickList: (uuid: string) => ['warehouse', 'order', uuid, 'pick-list'] as const,
  suggestions: (uuid: string, item: string, unlocated: boolean) =>
    ['warehouse', 'order', uuid, 'suggest', item, unlocated] as const,
  pickLocations: (uuid: string, item: string) => ['warehouse', 'order', uuid, 'locations', item] as const,
  unplaced: (uuid: string) => ['warehouse', 'order', uuid, 'unplaced'] as const,
  deliveryMethods: () => ['warehouse', 'delivery-methods'] as const,
};

export function useOrders(scope: OrderScope, enabled = true) {
  return useInfiniteQuery({
    queryKey: orderKeys.orders(scope),
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/orders', {
          params: { query: { ...SCOPES[scope], limit: PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
    placeholderData: keepPreviousData,
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
