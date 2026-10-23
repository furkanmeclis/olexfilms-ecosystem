import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { PAGE_SIZE, usePagedQuery } from '@/features/admin/kit';

import type { PurchasePaymentMethod, SalePaymentMethod } from './lib';

type S = components['schemas'];
export type PriceCatalogItem = S['DealerPriceCatalogItem'];
export type ProductSaleListItem = S['ProductSaleListItem'];
export type ProductSaleRequest = S['ProductSaleRequest'];
export type PurchaseListItem = S['PurchaseListItem'];
export type PurchaseRequest = S['PurchaseRequest'];
export type Supplier = S['Supplier'];

/**
 * Dealer sales (TEC-344/348, mobile TEC-958): the same endpoints as the web
 * screens, reached through the mobile mirror. The backend resolves the dealer
 * from the session; accounting + dealer_accounting modules and
 * dealer_pricing.write / product_sales.write / purchases.write /
 * suppliers.manage gate them.
 */
export const dealerSalesKeys = {
  all: ['dealer-sales'] as const,
  prices: (q: string, priced: string) => ['dealer-sales', 'prices', q, priced] as const,
  sales: (q: string, payment: string) => ['dealer-sales', 'sales', q, payment] as const,
  purchases: (q: string, payment: string) => ['dealer-sales', 'purchases', q, payment] as const,
  suppliers: ['dealer-sales', 'supplier-options'] as const,
  productSearch: (q: string) => ['dealer-sales', 'product-search', q] as const,
};

export type PricedFilter = '' | 'true' | 'false';

/** GET /v1/dealer-prices/catalog: the brand's products with the dealer's price. */
export function usePriceCatalog(q: string, priced: PricedFilter) {
  return usePagedQuery<PriceCatalogItem>(dealerSalesKeys.prices(q, priced), (offset) =>
    unwrap(
      api.GET('/v1/dealer-prices/catalog', {
        params: {
          query: {
            q: q || undefined,
            priced: priced === '' ? undefined : priced === 'true',
            sort: 'name',
            limit: PAGE_SIZE,
            offset,
          },
        },
      }),
    ),
  );
}

/** Product search of the quick sale (the price catalog, 10 rows). */
export function useProductSearch(q: string, enabled: boolean) {
  return useQuery({
    queryKey: dealerSalesKeys.productSearch(q),
    enabled,
    queryFn: () =>
      unwrap(api.GET('/v1/dealer-prices/catalog', { params: { query: { q: q || undefined, limit: 10, offset: 0 } } })),
  });
}

export function setDealerPrice(productUuid: string, salePrice: string) {
  return unwrap(api.PUT('/v1/dealer-prices', { body: { product_uuid: productUuid, sale_price: salePrice } }));
}

/** GET /v1/product-sales/lookup: the unit a sale would consume. */
export function lookupSaleItem(query: { barcode?: string; product_uuid?: string }) {
  return unwrap(api.GET('/v1/product-sales/lookup', { params: { query } }));
}

export function useProductSales(q: string, payment: '' | SalePaymentMethod) {
  return usePagedQuery<ProductSaleListItem>(dealerSalesKeys.sales(q, payment), (offset) =>
    unwrap(
      api.GET('/v1/product-sales', {
        params: { query: { q: q || undefined, payment_method: payment || undefined, sort: '-sold_at', limit: PAGE_SIZE, offset } },
      }),
    ),
  );
}

export function createSale(body: ProductSaleRequest) {
  return unwrap(api.POST('/v1/product-sales', { body }));
}

export function voidSale(uuid: string, reason: string) {
  return unwrap(api.POST('/v1/product-sales/{uuid}/void', { params: { path: { uuid } }, body: { reason } }));
}

export function usePurchases(q: string, payment: '' | PurchasePaymentMethod) {
  return usePagedQuery<PurchaseListItem>(dealerSalesKeys.purchases(q, payment), (offset) =>
    unwrap(
      api.GET('/v1/purchases', {
        params: {
          query: { q: q || undefined, payment_method: payment || undefined, sort: '-purchased_on', limit: PAGE_SIZE, offset },
        },
      }),
    ),
  );
}

/** Active suppliers for the purchase form (web: active, 100, by name). */
export function useSupplierOptions(enabled: boolean) {
  return useQuery({
    queryKey: dealerSalesKeys.suppliers,
    enabled,
    queryFn: () => unwrap(api.GET('/v1/suppliers', { params: { query: { active: true, limit: 100, offset: 0, sort: 'name' } } })),
  });
}

export function createPurchase(body: PurchaseRequest) {
  return unwrap(api.POST('/v1/purchases', { body }));
}
