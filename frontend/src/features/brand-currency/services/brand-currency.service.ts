import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type PlatformBrand = Schemas["PlatformBrand"];
export type BrandCurrencyChange = Schemas["BrandCurrencyChange"];
export type BrandCurrencyChangeRequest = Schemas["BrandCurrencyChangeRequest"];
export type BrandPriceConversion = Schemas["BrandPriceConversion"];
export type BrandPriceConversionRequest =
  Schemas["BrandPriceConversionRequest"];

export const brandCurrencyKeys = {
  all: ["platform", "brands"] as const,
  list: ["platform", "brands", "list"] as const,
};

/** TEC-1214: brand currency (platform). */
export const brandCurrencyService = {
  list() {
    return platformRequest<{ items: PlatformBrand[]; local_currency: string }>(
      "GET",
      "/v1/platform/brands",
    );
  },

  changeCurrency(uuid: string, body: BrandCurrencyChangeRequest) {
    return platformRequest<BrandCurrencyChange>(
      "PUT",
      `/v1/platform/brands/${uuid}/currency`,
      { body },
    );
  },

  convertPrices(uuid: string, body: BrandPriceConversionRequest) {
    return platformRequest<BrandPriceConversion>(
      "POST",
      `/v1/platform/brands/${uuid}/price-conversion`,
      { body },
    );
  },
};
