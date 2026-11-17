import { useSession } from '@/core/auth/session';

/**
 * TEC-1214: currencies come from the session (`/auth/me`), never from
 * constants.
 * - brand: price and order currency of the active brand.
 * - organization: ledger (cari / finance) currency of the active organization.
 * - display: local display currency (pricing.local_currency): the small
 *   converted amount next to brand-currency prices (live_total_try).
 */
export type SessionCurrencies = {
  brand: string | null;
  organization: string | null;
  display: string | null;
};

type CurrencyFields = {
  brand_currency?: string | null;
  organization_currency?: string | null;
  display_currency?: string | null;
};

export function currenciesOf(me: CurrencyFields | null | undefined): SessionCurrencies {
  return {
    brand: me?.brand_currency || null,
    organization: me?.organization_currency || null,
    display: me?.display_currency || null,
  };
}

export function useSessionCurrencies(): SessionCurrencies {
  const me = useSession((s) => s.me);
  return currenciesOf(me);
}
