import { apiConfig } from "@/config/api";
import type { LatLng } from "@/config/map";
import {
  parseRetryAfter,
  type UpstreamFetcher,
} from "@/features/warranty/lib/public-warranty";
import type { components } from "@/generated/api";

/**
 * Public dealer showcase (TEC-250): `/bayi/{code}` reads Go
 * `GET /v1/public/dealers/{code}` on the server. `code` is the
 * organization slug; the response carries showcase fields only.
 */
export type PublicDealer = components["schemas"]["PublicDealer"];

/** Same shape as the Go check (organization slug); else 404 without a call. */
export const DEALER_CODE_RE = /^[a-z0-9][a-z0-9-]{0,99}$/;

export type PublicDealerResult =
  | { kind: "ok"; dealer: PublicDealer }
  | { kind: "not_found" }
  | { kind: "rate_limited"; retryAfter: number | null }
  | { kind: "error" };

export type PublicDealerRequest = {
  /** Browser IP: the Go per-IP rate limit keys on it. */
  clientIp: string | null;
  /** Browser host: Go resolves the brand from it (K3, K20). */
  forwardedHost: string | null;
};

/** Lower-cased, trimmed code; null when it cannot be a dealer slug. */
export function normalizeDealerCode(code: string): string | null {
  let decoded = code;
  try {
    decoded = decodeURIComponent(code);
  } catch {
    return null;
  }
  const value = decoded.trim().toLowerCase();
  return DEALER_CODE_RE.test(value) ? value : null;
}

/**
 * Looks a dealer up. A malformed code is not_found without a request (Go
 * answers the same 404); 429 keeps Retry-After; anything else unexpected is
 * error.
 */
export async function fetchPublicDealer(
  code: string,
  request: PublicDealerRequest,
  fetcher: UpstreamFetcher,
): Promise<PublicDealerResult> {
  const normalized = normalizeDealerCode(code);
  if (!normalized) return { kind: "not_found" };
  const headers = new Headers({ Accept: "application/json" });
  if (request.clientIp) headers.set("X-Forwarded-For", request.clientIp);
  if (request.forwardedHost)
    headers.set("X-Forwarded-Host", request.forwardedHost);

  let res: Awaited<ReturnType<UpstreamFetcher>>;
  try {
    res = await fetcher(`public/dealers/${encodeURIComponent(normalized)}`, {
      method: "GET",
      headers,
    });
  } catch {
    return { kind: "error" };
  }
  if (res.status === 404) return { kind: "not_found" };
  if (res.status === 429) {
    return {
      kind: "rate_limited",
      retryAfter: parseRetryAfter(res.headers.get("retry-after")),
    };
  }
  if (res.status !== 200) return { kind: "error" };
  try {
    const envelope = JSON.parse(new TextDecoder().decode(res.body)) as {
      data?: PublicDealer;
    };
    if (!envelope.data?.code || !envelope.data.name) return { kind: "error" };
    return { kind: "ok", dealer: envelope.data };
  } catch {
    return { kind: "error" };
  }
}

/** Map position of the dealer, or null (no map is shown). */
export function dealerPosition(dealer: PublicDealer): LatLng | null {
  const { latitude: lat, longitude: lng } = dealer;
  if (typeof lat !== "number" || typeof lng !== "number") return null;
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return null;
  return { lat, lng };
}

/** "District, City" without empty parts. */
export function dealerLocality(dealer: PublicDealer): string {
  return [dealer.district, dealer.city]
    .map((part) => part?.trim())
    .filter(Boolean)
    .join(", ");
}

/** Browser URL of the dealer logo (through the BFF), or null. */
export function dealerLogoSrc(dealer: PublicDealer): string | null {
  if (!dealer.logo_url) return null;
  return `${apiConfig.baseUrl.replace(/\/$/, "")}${dealer.logo_url}`;
}

/** OpenStreetMap link of the position (directions on any device). */
export function dealerMapHref(position: LatLng): string {
  const lat = position.lat.toFixed(6);
  const lng = position.lng.toFixed(6);
  return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=17/${lat}/${lng}`;
}
