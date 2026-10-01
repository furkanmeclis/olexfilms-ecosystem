import type { components } from "@/generated/api";
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

export type ListCategoriesParams = {
  limit?: number;
  offset?: number;
  q?: string;
  active?: boolean;
};

export type ListProductsParams = {
  limit?: number;
  offset?: number;
  q?: string;
  active?: boolean;
  category_uuid?: string;
  unit_type?: CatalogUnitType;
};

export type BulkActiveResult = {
  requested: number;
  updated: number;
  uuids: string[];
};

/** At most this many uuids per bulk-active request (TEC-145). */
export const BULK_ACTIVE_MAX = 500;

function boolQuery(value: boolean | undefined) {
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
          active: boolQuery(params.active),
        },
      },
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
    return platformRequest<CatalogPage<CatalogProduct>>(
      "GET",
      "/v1/catalog/products",
      {
        query: {
          limit: params.limit,
          offset: params.offset,
          q: params.q,
          active: boolQuery(params.active),
          category_uuid: params.category_uuid,
          unit_type: params.unit_type,
        },
      },
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

  bulkActive(uuids: string[], active: boolean) {
    return platformRequest<BulkActiveResult>(
      "POST",
      "/v1/catalog/products/bulk-active",
      { body: { uuids: uuids.slice(0, BULK_ACTIVE_MAX), active } },
    );
  },
};
