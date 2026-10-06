import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import type { CatalogPage } from "@/features/catalog/services/catalog.service";

type Schemas = components["schemas"];

export type EffectivePrice = Schemas["EffectivePrice"];
export type ProductPriceView = Schemas["ProductPriceView"];
export type PriceViewer = ProductPriceView["viewer"];
export type ListPriceInput = Schemas["ListPriceInput"];
export type DistributorPrice = Schemas["DistributorPrice"];

export type DistributorOption = { uuid: string; name: string };

/**
 * `GET /v1/tenant/pricing/distributor-prices` query (TEC-369): paging,
 * `sort` (product, distributor, currency, price, updated_at), `q` (SKU,
 * product or distributor name) and `currency` (ISO-4217 CSV).
 */
export type DistributorPriceListParams = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
  currency?: string;
};

const enc = encodeURIComponent;

/**
 * Tenant pricing API (TEC-146). Reads come back masked per caller; every
 * write needs pricing.sale.write plus a recent step-up, which
 * platformRequest handles through the step-up engine (withStepUpRetry).
 */
export const pricingService = {
  getProduct(uuid: string) {
    return platformRequest<ProductPriceView>(
      "GET",
      `/v1/tenant/pricing/products/${enc(uuid)}`,
    );
  },

  setListPrice(uuid: string, currency: string, body: ListPriceInput) {
    return platformRequest<ProductPriceView>(
      "PUT",
      `/v1/tenant/pricing/products/${enc(uuid)}/prices/${enc(currency)}`,
      { body },
    );
  },

  deleteListPrice(uuid: string, currency: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/tenant/pricing/products/${enc(uuid)}/prices/${enc(currency)}`,
    );
  },

  listDistributorPrices(
    productUuid: string,
    params: DistributorPriceListParams = { limit: 100 },
  ) {
    return platformRequest<CatalogPage<DistributorPrice>>(
      "GET",
      "/v1/tenant/pricing/distributor-prices",
      { query: { product_uuid: productUuid, ...params } },
    );
  },

  setDistributorPrice(
    uuid: string,
    distributorUuid: string,
    currency: string,
    price: string,
  ) {
    return platformRequest<DistributorPrice>(
      "PUT",
      `/v1/tenant/pricing/products/${enc(uuid)}/distributor-prices/${enc(distributorUuid)}/${enc(currency)}`,
      { body: { price } },
    );
  },

  deleteDistributorPrice(
    uuid: string,
    distributorUuid: string,
    currency: string,
  ) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/tenant/pricing/products/${enc(uuid)}/distributor-prices/${enc(distributorUuid)}/${enc(currency)}`,
    );
  },

  setDealerPrice(uuid: string, currency: string, price: string) {
    return platformRequest<ProductPriceView>(
      "PUT",
      `/v1/tenant/pricing/products/${enc(uuid)}/dealer-prices/${enc(currency)}`,
      { body: { price } },
    );
  },

  deleteDealerPrice(uuid: string, currency: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/tenant/pricing/products/${enc(uuid)}/dealer-prices/${enc(currency)}`,
    );
  },

  /** Distributors of the brand, for the distributor-price picker. */
  async listDistributors(): Promise<DistributorOption[]> {
    const data = await platformRequest<{
      items: { uuid: string; name: string }[];
    }>("GET", "/v1/tenant/organizations", {
      query: { type: "distributor", limit: 200 },
    });
    return (data.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name }));
  },
};
