"use client";

/**
 * TEC-1214: currencies come from the session and the API, never from
 * constants in the client.
 *
 * - brand: price and order currency of the active brand (prices, order
 *   totals, price lists).
 * - organization: ledger currency of the active organization (cari,
 *   finance).
 * - display: local display currency (pricing.local_currency): the small
 *   converted amount next to brand-currency prices (live_total_try).
 */
import { useQuery } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import { useAuthStore } from "@/lib/auth/session-store";

export type Currency = components["schemas"]["Currency"];

export type SessionCurrencies = {
  brand: string | null;
  organization: string | null;
  display: string | null;
};

export function useSessionCurrencies(): SessionCurrencies {
  const user = useAuthStore((s) => s.user);
  return {
    brand: user?.brandCurrency ?? null,
    organization: user?.organizationCurrency ?? null,
    display: user?.displayCurrency ?? null,
  };
}

export const currencyKeys = {
  list: ["currencies", "active"] as const,
};

/** Active currencies (GET /v1/currencies), cached for an hour. */
export function useCurrencyList(enabled = true) {
  return useQuery({
    queryKey: currencyKeys.list,
    queryFn: () =>
      platformRequest<{ items: Currency[] }>("GET", "/v1/currencies"),
    enabled,
    staleTime: 60 * 60 * 1000,
    select: (data) => data.items.map((c) => c.code),
  });
}
