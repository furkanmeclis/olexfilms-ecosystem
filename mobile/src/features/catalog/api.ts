import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { API_URL } from '@/core/config';
import { PAGE_SIZE, usePagedQuery } from '@/features/admin/kit';

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
