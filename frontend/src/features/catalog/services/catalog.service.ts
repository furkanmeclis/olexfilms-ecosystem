import type { components } from "@/generated/api";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type CatalogCategory = Schemas["CatalogCategory"];
export type CatalogCategoryInput = Schemas["CatalogCategoryInput"];
export type CatalogProduct = Schemas["CatalogProduct"];
export type CatalogProductInput = Schemas["CatalogProductInput"];
export type CatalogProductImage = Schemas["CatalogProductImage"];
export type CatalogUnitType = Schemas["CatalogUnitType"];

export const CATALOG_UNIT_TYPES: CatalogUnitType[] = ["piece", "roll_meter"];

export type CatalogPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * List query of `GET /v1/catalog/categories` (TEC-369): paging, `sort`
 * (name, sort, active, created_at, updated_at), `q` and `active=true|false`.
 */
export type ListCategoriesParams = {
  limit?: number;
  offset?: number;
  q?: string;
  sort?: string;
  active?: boolean | string;
};

/**
 * List query of `GET /v1/catalog/products` (TEC-369): paging, single-field
 * `sort`, `q` and the column filters (`active`, `category_uuid` CSV,
 * `unit_type` CSV, `uses_fixed_barcode`, warranty / micron `_min`/`_max`,
 * `created_from` / `created_to`), passed through as query params.
 */
export type ListProductsParams = {
  limit?: number;
  offset?: number;
  q?: string;
  sort?: string;
} & Record<string, string | number | boolean | undefined>;

export type BulkActiveResult = {
  requested: number;
  updated: number;
  uuids: string[];
};

/** Product image limits (TEC-152); the backend enforces the same. */
export const PRODUCT_IMAGE_MAX = 10;
export const PRODUCT_IMAGE_MAX_BYTES = 5 * 1024 * 1024;
export const PRODUCT_IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];

/** Keys issued by the upload route (32 hex + raster extension). */
const UPLOADED_IMAGE_KEY = /^[0-9a-f]{32}\.(jpg|png|webp)$/;

/** Public, cacheable URL of an uploaded product image; null for legacy keys. */
export function productImageUrl(key: string): string | null {
  return UPLOADED_IMAGE_KEY.test(key)
    ? `/product-images/${encodeURIComponent(key)}`
    : null;
}

/** At most this many uuids per bulk-active request (TEC-145). */
export const BULK_ACTIVE_MAX = 500;

function boolQuery(value: boolean | string | undefined) {
  return value === undefined ? undefined : String(value);
}

/** Tenant catalog API (TEC-145): brand-scoped, writes center only (K4). */
export const catalogService = {
  listCategories(params: ListCategoriesParams = {}) {
    return platformRequest<CatalogPage<CatalogCategory>>(
      "GET",
      "/v1/catalog/categories",
      {
        query: {
          limit: params.limit,
          offset: params.offset,
          q: params.q,
          sort: params.sort,
          active: boolQuery(params.active),
        },
      },
    );
  },

  /**
   * Center only (TEC-369): rearranges the given categories within the
   * positions they hold and renumbers every category; returns the full list.
   */
  reorderCategories(uuids: string[]) {
    return platformRequest<{ items: CatalogCategory[] }>(
      "PUT",
      "/v1/catalog/categories/order",
      { body: { uuids } },
    );
  },

  createCategory(body: CatalogCategoryInput) {
    return platformRequest<CatalogCategory>("POST", "/v1/catalog/categories", {
      body,
    });
  },

  updateCategory(uuid: string, body: CatalogCategoryInput) {
    return platformRequest<CatalogCategory>(
      "PATCH",
      `/v1/catalog/categories/${encodeURIComponent(uuid)}`,
      { body },
    );
  },

  deleteCategory(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/catalog/categories/${encodeURIComponent(uuid)}`,
    );
  },

  listProducts(params: ListProductsParams = {}) {
    const query: Record<string, string | number | undefined> = {};
    for (const [key, value] of Object.entries(params)) {
      if (value === undefined || value === "") continue;
      query[key] = typeof value === "boolean" ? String(value) : value;
    }
    return platformRequest<CatalogPage<CatalogProduct>>(
      "GET",
      "/v1/catalog/products",
      { query },
    );
  },

  getProduct(uuid: string) {
    return platformRequest<CatalogProduct>(
      "GET",
      `/v1/catalog/products/${encodeURIComponent(uuid)}`,
    );
  },

  createProduct(body: CatalogProductInput) {
    return platformRequest<CatalogProduct>("POST", "/v1/catalog/products", {
      body,
    });
  },

  updateProduct(uuid: string, body: CatalogProductInput) {
    return platformRequest<CatalogProduct>(
      "PATCH",
      `/v1/catalog/products/${encodeURIComponent(uuid)}`,
      { body },
    );
  },

  deleteProduct(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/catalog/products/${encodeURIComponent(uuid)}`,
    );
  },

  uploadProductImage(uuid: string, file: File) {
    const form = new FormData();
    form.append("image", file);
    return platformFormRequest<CatalogProduct>(
      "POST",
      `/v1/catalog/products/${encodeURIComponent(uuid)}/images`,
      form,
    );
  },

  deleteProductImage(uuid: string, key: string) {
    return platformRequest<CatalogProduct>(
      "DELETE",
      `/v1/catalog/products/${encodeURIComponent(uuid)}/images/${encodeURIComponent(key)}`,
    );
  },

  reorderProductImages(uuid: string, keys: string[]) {
    return platformRequest<CatalogProduct>(
      "PUT",
      `/v1/catalog/products/${encodeURIComponent(uuid)}/images/order`,
      { body: { keys } },
    );
  },

  bulkActive(uuids: string[], active: boolean) {
    return platformRequest<BulkActiveResult>(
      "POST",
      "/v1/catalog/products/bulk-active",
      { body: { uuids: uuids.slice(0, BULK_ACTIVE_MAX), active } },
    );
  },
};
