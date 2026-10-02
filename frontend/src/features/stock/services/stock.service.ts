import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type StockUnitRow = Schemas["StockUnitRow"];
export type StockUnitStatus = Schemas["StockUnitStatus"];
export type StockProduct = Schemas["StockProduct"];

/** GET /v1/stock/organizations/{uuid}/units filters (TEC-216). */
export type StockUnitListQuery = {
  q?: string;
  product_uuid?: string;
  status?: StockUnitStatus;
  barcode?: string;
  limit: number;
  offset: number;
};

/** GET /v1/stock/organizations/{uuid}/products filters. */
export type StockProductListQuery = {
  q?: string;
  product_uuid?: string;
  status?: "in_stock" | "out_of_stock";
  limit: number;
  offset: number;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** A dealer the viewer may pick (distributor subtree, K4). */
export type StockDealerOption = { uuid: string; name: string };

const enc = encodeURIComponent;

/**
 * Dealer "My stock" reads (TEC-224, K12): the units and the product
 * projection of one organization. A distributor passes a dealer of its
 * subtree (stock.read at scope subtree, TEC-216) and reads it as is.
 */
export const stockService = {
  listUnits(orgUuid: string, params: StockUnitListQuery) {
    return platformRequest<Page<StockUnitRow>>(
      "GET",
      `/v1/stock/organizations/${enc(orgUuid)}/units`,
      { query: params },
    );
  },
  listProducts(orgUuid: string, params: StockProductListQuery) {
    return platformRequest<Page<StockProduct>>(
      "GET",
      `/v1/stock/organizations/${enc(orgUuid)}/products`,
      { query: params },
    );
  },
  /** Dealers in the organizations.read scope (a distributor's subtree). */
  async listDealers(): Promise<StockDealerOption[]> {
    const data = await platformRequest<{ items: StockDealerOption[] }>(
      "GET",
      "/v1/tenant/organizations",
      { query: { type: "dealer", limit: 200 } },
    );
    return (data.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name }));
  },
};

export const stockKeys = {
  all: ["stock"] as const,
  units: (orgUuid: string, params: StockUnitListQuery) =>
    ["stock", "units", orgUuid, params] as const,
  products: (orgUuid: string, params: StockProductListQuery) =>
    ["stock", "products", orgUuid, params] as const,
  dealers: ["stock", "dealers"] as const,
};
