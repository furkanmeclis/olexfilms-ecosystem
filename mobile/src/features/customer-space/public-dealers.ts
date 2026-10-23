import type { components } from '@/core/api/schema';
import { ApiError } from '@/core/api/errors';
import { API_URL, APP_VERSION } from '@/core/config';
import { i18n } from '@/core/i18n';

import { parseRetryAfter, PUBLIC_DEALER_CENTER, PUBLIC_DEALER_RADIUS_KM } from './booking-lib';

export type NearbyDealer = components['schemas']['NearbyDealer'];

/** Backend cap of the public list: NearbyMaxResults in backend/internal/modules/organizations/usecase/dealers.go. */
export const NEARBY_MAX_RESULTS = 50;

/** A 429 waits at most this long before the single retry. */
const MAX_RETRY_WAIT_S = 5;

/**
 * Public dealer list of the brand (GET /v1/public/dealers/nearby): no token and
 * not through the panel / portal mirror, like the catalog's public image URLs.
 * Without device location the web's default centre and widest radius are used.
 */
export async function fetchPublicDealers(signal?: AbortSignal): Promise<NearbyDealer[]> {
  const url =
    `${API_URL}/v1/public/dealers/nearby?lat=${PUBLIC_DEALER_CENTER.lat}` +
    `&lng=${PUBLIC_DEALER_CENTER.lng}&radius_km=${PUBLIC_DEALER_RADIUS_KM}`;
  for (let attempt = 0; ; attempt++) {
    let res: Response;
    try {
      res = await fetch(url, {
        signal,
        headers: { Accept: 'application/json', 'Accept-Language': i18n.language, 'X-App-Version': APP_VERSION },
      });
    } catch {
      throw new ApiError(0);
    }
    if (res.status === 429 && attempt === 0) {
      const wait = Math.min(parseRetryAfter(res.headers.get('Retry-After')) ?? 1, MAX_RETRY_WAIT_S);
      await new Promise((r) => setTimeout(r, wait * 1000));
      continue;
    }
    if (!res.ok) {
      let body: { error?: Partial<components['schemas']['ErrorBody']> } | undefined;
      try {
        body = await res.json();
      } catch {
        body = undefined;
      }
      throw new ApiError(res.status, res.status === 429 ? { code: 'RATE_LIMITED', ...body?.error } : body?.error);
    }
    const body = (await res.json()) as { data?: { items?: NearbyDealer[] } };
    return body.data?.items ?? [];
  }
}
