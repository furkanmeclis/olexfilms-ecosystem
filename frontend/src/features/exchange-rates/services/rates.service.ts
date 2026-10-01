import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Currency = Schemas["Currency"];
export type StoredExchangeRate = Schemas["StoredExchangeRate"];
export type RateSnapshot = Schemas["RateSnapshot"];
export type FetchSourceReport = Schemas["FetchSourceReport"];
export type ExchangeRateOverrideRequest =
  Schemas["ExchangeRateOverrideRequest"];

export const ratesService = {
  currencies() {
    return platformRequest<{ items: Currency[] }>("GET", "/v1/currencies");
  },

  day(date?: string, base?: string) {
    return platformRequest<{ date: string; items: StoredExchangeRate[] }>(
      "GET",
      "/v1/platform/exchange-rates",
      { query: { date, base } },
    );
  },

  resolve(date: string, base: string, quote: string) {
    return platformRequest<RateSnapshot>(
      "GET",
      "/v1/platform/exchange-rates/resolve",
      { query: { date, base, quote } },
    );
  },

  setOverride(body: ExchangeRateOverrideRequest) {
    return platformRequest<RateSnapshot>(
      "PUT",
      "/v1/platform/exchange-rates/override",
      { body },
    );
  },

  clearOverride(date: string, base: string, quote: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      "/v1/platform/exchange-rates/override",
      { query: { date, base, quote } },
    );
  },

  fetchNow() {
    return platformRequest<{ sources: FetchSourceReport[] }>(
      "POST",
      "/v1/platform/exchange-rates/fetch",
    );
  },
};
