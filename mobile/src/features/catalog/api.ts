import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { API_URL } from '@/core/config';
import { PAGE_SIZE, usePagedQuery } from '@/features/admin/kit';
import { stockByProduct } from '@/features/orders/order-summary';

type S = components['schemas'];
export type Product = S['CatalogProduct'];
export type Category = S['CatalogCategory'];

export const catalogKeys = {
  categories: () => ['catalog', 'categories'] as const,
  products: (category: string, q: string) => ['catalog', 'products', category, q] as const,
  product: (uuid: string) => ['catalog', 'product', uuid] as const,
};

/** Only uploaded images have a public URL (same rule as the web, TEC-152). */
const UPLOADED_IMAGE_KEY = /^[0-9a-f]{32}\.(jpg|png|webp)$/;

/** Public, cacheable product image (no token): GET /v1/public/product-images/{key}. */
export function productImageUrl(key: string | undefined): string | null {
  if (!key || !UPLOADED_IMAGE_KEY.test(key)) return null;
  return `${API_URL}/v1/public/product-images/${encodeURIComponent(key)}`;
}

export function firstImage(p: Product) {
  return productImageUrl([...p.images].sort((a, b) => a.sort - b.sort)[0]?.key);
}

export function useCategories() {
  return useQuery({
    queryKey: catalogKeys.categories(),
    queryFn: () => unwrap(api.GET('/v1/catalog/categories', { params: { query: { active: true, limit: 100 } } })),
    select: (d) => [...d.items].sort((a, b) => a.sort - b.sort || a.name.localeCompare(b.name)),
    staleTime: 5 * 60_000,
  });
}

/** Active products of the brand catalog, by category and name / SKU. */
export function useProducts(category: string, q: string) {
  return usePagedQuery<Product>(catalogKeys.products(category, q), (offset) =>
    unwrap(
      api.GET('/v1/catalog/products', {
        params: { query: { active: true, category_uuid: category || undefined, q: q || undefined, limit: PAGE_SIZE, offset } },
      }),
    ),
  );
}

export function useProduct(uuid: string) {
  return useQuery({
    queryKey: catalogKeys.product(uuid),
    queryFn: () => unwrap(api.GET('/v1/catalog/products/{uuid}', { params: { path: { uuid } } })),
  });
}

/** Stock rows read for the catalog marks: 100 per page, at most this many pages. */
const STOCK_PAGE = 100;
const STOCK_PAGES = 5;

/**
 * On hand stock per product of one organization (TEC-1055, revize 5.1), for
 * the "in stock / out of stock" mark of the catalog rows. Only products in
 * stock are read; `complete` is false when there were more than the pages
 * read, then a product missing from the map is unknown, not out of stock.
 */
export function useCatalogStock(org: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ['catalog', 'stock', org ?? ''],
    enabled: enabled && !!org,
    staleTime: 60_000,
    queryFn: async () => {
      const rows: S['StockProduct'][] = [];
      let total = 0;
      for (let page = 0; page < STOCK_PAGES; page++) {
        const res = await unwrap(
          api.GET('/v1/stock/organizations/{uuid}/products', {
            params: { path: { uuid: org! }, query: { status: 'in_stock', limit: STOCK_PAGE, offset: page * STOCK_PAGE } },
          }),
        );
        rows.push(...res.items);
        total = res.total;
        if (rows.length >= total || !res.items.length) break;
      }
      return { map: stockByProduct(rows), complete: rows.length >= total };
    },
  });
}
