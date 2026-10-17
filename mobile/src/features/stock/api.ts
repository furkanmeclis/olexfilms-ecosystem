import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

type S = components['schemas'];
export type StockProduct = S['StockProduct'];
export type StockUnitRow = S['StockUnitRow'];
export type StockUnitHistory = S['StockUnitHistory'];

const PAGE = 30;

type Page<T> = { items: T[]; total: number; limit: number; offset: number };
const next = <T,>(last: Page<T>) =>
  last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined;

/** Stock of an organization by product (GET /v1/stock/organizations/{uuid}/products). */
export function useOrgProducts(org: string, q: string, status: '' | 'in_stock' | 'out_of_stock') {
  return useInfiniteQuery({
    queryKey: ['stock', 'products', org, q, status],
    enabled: !!org,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/stock/organizations/{uuid}/products', {
          params: {
            path: { uuid: org },
            query: { q: q || undefined, status: status || undefined, limit: PAGE, offset: pageParam, sort: 'product' },
          },
        }),
      ),
    getNextPageParam: next,
    placeholderData: keepPreviousData,
  });
}

/** Units of one product in the organization, with their locations. */
export function useProductUnits(org: string, product: string) {
  return useInfiniteQuery({
    queryKey: ['stock', 'units', org, product],
    enabled: !!org && !!product,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/stock/organizations/{uuid}/units', {
          params: { path: { uuid: org }, query: { product_uuid: product, limit: PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: next,
  });
}

/** Products on one location (and below). */
export function useLocationProducts(location: string) {
  return useInfiniteQuery({
    queryKey: ['stock', 'location', location],
    enabled: !!location,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/stock/locations/{uuid}/products', {
          params: { path: { uuid: location }, query: { limit: PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: next,
  });
}

/** Barcode history: unit, current holder and movements. */
export function useUnitHistory(barcode: string) {
  return useQuery({
    queryKey: ['stock', 'unit', barcode],
    enabled: !!barcode,
    queryFn: () =>
      unwrap(
        api.GET('/v1/stock/units/by-barcode/{barcode}', {
          params: { path: { barcode }, query: { limit: 50 } },
        }),
      ),
  });
}
