import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformDownloadFile } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type StockUnitRow = Schemas["StockUnitRow"];
export type StockUnitStatus = Schemas["StockUnitStatus"];
export type StockProduct = Schemas["StockProduct"];

/**
 * GET /v1/stock/organizations/{uuid}/units params (TEC-216, TEC-373): the
 * list contract (`sort` product/barcode/status/quantity/meters/updated_at,
 * `q`, CSV `status` / `location_uuid`, `product_uuid`, `barcode` with
 * `barcode_match=exact|prefix`, `updated_from` / `updated_to`).
 */
export type StockUnitListQuery = ServerListQuery;

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

/** At most this many labels in one units.pdf request (backend cap). */
export const MAX_UNIT_LABELS = 1000;

/** POST .../units/export of an organization (TEC-373): csv, xlsx or pdf. */
export function stockUnitsExportPath(orgUuid: string): string {
  return `/v1/stock/organizations/${encodeURIComponent(orgUuid)}/units/export`;
}

/**
 * GET /v1/stock/labels/units.pdf with one `barcode` per unit (TEC-373:
 * labels for the selected units).
 */
export function unitLabelsPath(barcodes: readonly string[]): string {
  const query = barcodes
    .slice(0, MAX_UNIT_LABELS)
    .map((b) => `barcode=${encodeURIComponent(b)}`)
    .join("&");
  return `/v1/stock/labels/units.pdf?${query}`;
}

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
  /** Label sheet PDF of the given unit barcodes. */
  unitLabels(barcodes: readonly string[]) {
    return platformDownloadFile(unitLabelsPath(barcodes));
  },
  /** Dealers in the organizations.read scope (a distributor's subtree). */
  async listDealers(): Promise<StockDealerOption[]> {
    const data = await platformRequest<{ items: StockDealerOption[] }>(
      "GET",
      "/v1/tenant/organizations",
      // The list endpoint caps `limit` at 100 (apiquery).
      { query: { type: "dealer", limit: 100 } },
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
