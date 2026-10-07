import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type PriceCatalogItem = Schemas["DealerPriceCatalogItem"];
export type DealerPrice = Schemas["DealerPrice"];
export type SaleLookup = Schemas["ProductSaleLookup"];
export type ProductSale = Schemas["ProductSale"];
export type ProductSaleRequest = Schemas["ProductSaleRequest"];
export type ProductSaleListItem = Schemas["ProductSaleListItem"];
export type Supplier = Schemas["Supplier"];
export type SupplierRequest = Schemas["SupplierRequest"];
export type Purchase = Schemas["Purchase"];
export type PurchaseRequest = Schemas["PurchaseRequest"];
export type PurchaseListItem = Schemas["PurchaseListItem"];

export type SalePaymentMethod = ProductSaleRequest["payment_method"];
export type PurchasePaymentMethod = PurchaseRequest["payment_method"];

export const SALE_PAYMENT_METHODS: SalePaymentMethod[] = [
  "cash",
  "card",
  "cari",
];
export const PURCHASE_PAYMENT_METHODS: PurchasePaymentMethod[] = [
  "cash",
  "card",
  "bank_transfer",
  "cari",
];

export type DealerSalesPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** List query params (limit, offset, sort, q and the mapped filters). */
export type ListQuery = Record<string, string | number | boolean | undefined>;

function listQuery(params: ListQuery) {
  const out: Record<string, string | number | undefined> = {};
  for (const [key, value] of Object.entries(params)) {
    out[key] = typeof value === "boolean" ? String(value) : value;
  }
  return out;
}

/**
 * Dealer sales API (TEC-344 writes, TEC-348 lists). The backend resolves
 * the dealer from the session context; every call works on its own book.
 */
export const dealerSalesService = {
  /**
   * GET /v1/dealer-prices/catalog: sort name (default), sku, sale_price,
   * recommended_sale_price, updated_at; q (name, sku); priced.
   */
  listPrices(params: ListQuery = {}) {
    return platformRequest<DealerSalesPage<PriceCatalogItem>>(
      "GET",
      "/v1/dealer-prices/catalog",
      { query: listQuery(params) },
    );
  },

  setPrice(productUuid: string, salePrice: string) {
    return platformRequest<DealerPrice>("PUT", "/v1/dealer-prices", {
      body: { product_uuid: productUuid, sale_price: salePrice },
    });
  },

  /** GET /v1/product-sales/lookup: the unit a sale would consume. */
  lookup(params: { barcode?: string; product_uuid?: string }) {
    return platformRequest<SaleLookup>("GET", "/v1/product-sales/lookup", {
      query: params,
    });
  },

  /**
   * GET /v1/product-sales: sort sold_at (default -sold_at), total,
   * payment_method; q; CSV payment_method; voided; sold_from / sold_to;
   * total_min / total_max.
   */
  listSales(params: ListQuery = {}) {
    return platformRequest<DealerSalesPage<ProductSaleListItem>>(
      "GET",
      "/v1/product-sales",
      { query: listQuery(params) },
    );
  },

  createSale(body: ProductSaleRequest) {
    return platformRequest<ProductSale>("POST", "/v1/product-sales", {
      body,
    });
  },

  voidSale(uuid: string, reason: string) {
    return platformRequest<ProductSale>(
      "POST",
      `/v1/product-sales/${encodeURIComponent(uuid)}/void`,
      { body: { reason } },
    );
  },

  /**
   * GET /v1/suppliers: sort name (default), created_at, updated_at; q
   * (name, tax no, phone, email); active; created_from / created_to.
   */
  listSuppliers(params: ListQuery = {}) {
    return platformRequest<DealerSalesPage<Supplier>>("GET", "/v1/suppliers", {
      query: listQuery(params),
    });
  },

  createSupplier(body: SupplierRequest) {
    return platformRequest<Supplier>("POST", "/v1/suppliers", { body });
  },

  updateSupplier(uuid: string, body: SupplierRequest) {
    return platformRequest<Supplier>(
      "PATCH",
      `/v1/suppliers/${encodeURIComponent(uuid)}`,
      { body },
    );
  },

  /**
   * GET /v1/purchases: sort purchased_on (default -purchased_on), amount,
   * supplier_name, payment_method; q; CSV supplier_uuid / payment_method;
   * purchased_from / purchased_to; amount_min / amount_max.
   */
  listPurchases(params: ListQuery = {}) {
    return platformRequest<DealerSalesPage<PurchaseListItem>>(
      "GET",
      "/v1/purchases",
      { query: listQuery(params) },
    );
  },

  createPurchase(body: PurchaseRequest) {
    return platformRequest<Purchase>("POST", "/v1/purchases", { body });
  },
};

export const dealerSalesKeys = {
  all: (org: string) => ["dealer-sales", org] as const,
  prices: (org: string, params: unknown) =>
    ["dealer-sales", org, "prices", params] as const,
  sales: (org: string, params: unknown) =>
    ["dealer-sales", org, "sales", params] as const,
  suppliers: (org: string, params: unknown) =>
    ["dealer-sales", org, "suppliers", params] as const,
  supplierOptions: (org: string) =>
    ["dealer-sales", org, "supplier-options"] as const,
  purchases: (org: string, params: unknown) =>
    ["dealer-sales", org, "purchases", params] as const,
};
