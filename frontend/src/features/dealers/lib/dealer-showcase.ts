import { apiConfig } from "@/config/api";
import type { AppLocale } from "@/config/i18n";
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
export type PublicDealerShowcase = NonNullable<PublicDealer["showcase"]>;
export type PublicDealerLeadFormConfig =
  components["schemas"]["PublicDealerLeadFormConfig"];
export type PublicDealerLeadRequest =
  components["schemas"]["PublicDealerLeadRequest"];

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
  /** Showcase text locale. */
  locale?: AppLocale;
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
    const query = request.locale
      ? `?${new URLSearchParams({ locale: request.locale })}`
      : "";
    res = await fetcher(
      `public/dealers/${encodeURIComponent(normalized)}${query}`,
      {
        method: "GET",
        headers,
      },
    );
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

export type LeadFormConfigResult =
  { kind: "ok"; config: PublicDealerLeadFormConfig } | { kind: "unavailable" };

export async function fetchLeadFormConfig(
  code: string,
  locale: AppLocale,
  fetchImpl: typeof fetch = fetch,
): Promise<LeadFormConfigResult> {
  const normalized = normalizeDealerCode(code);
  if (!normalized) return { kind: "unavailable" };
  try {
    const params = new URLSearchParams({ lang: locale });
    const res = await fetchImpl(
      `/api/v1/public/dealers/${encodeURIComponent(normalized)}/lead-form/config?${params}`,
      {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      },
    );
    if (!res.ok) return { kind: "unavailable" };
    const body = (await res.json()) as { data?: PublicDealerLeadFormConfig };
    if (!body.data?.form_token) return { kind: "unavailable" };
    return { kind: "ok", config: body.data };
  } catch {
    return { kind: "unavailable" };
  }
}

export type LeadSubmitResult =
  | { kind: "ok" }
  | { kind: "validation"; fields: string[] }
  | { kind: "rate_limited"; retryAfter: number | null }
  | { kind: "error" };

export async function submitPublicDealerLead(
  code: string,
  payload: PublicDealerLeadRequest,
  fetchImpl: typeof fetch = fetch,
): Promise<LeadSubmitResult> {
  const normalized = normalizeDealerCode(code);
  if (!normalized) return { kind: "error" };
  try {
    const res = await fetchImpl(
      `/api/v1/public/dealers/${encodeURIComponent(normalized)}/leads`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      },
    );
    if (res.status === 202 || res.status === 200 || res.status === 204) {
      return { kind: "ok" };
    }
    if (res.status === 429) {
      return {
        kind: "rate_limited",
        retryAfter: parseRetryAfter(res.headers.get("retry-after")),
      };
    }
    if (res.status === 400) {
      const body = (await res.json().catch(() => null)) as {
        error?: { details?: { field?: string }[] };
      } | null;
      return {
        kind: "validation",
        fields: (body?.error?.details ?? [])
          .map((d) => d.field)
          .filter((f): f is string => Boolean(f)),
      };
    }
    return { kind: "error" };
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

/** Browser URL of a public showcase photo. */
export function dealerPhotoSrc(url: string): string {
  return url.startsWith("http")
    ? url
    : `${apiConfig.baseUrl.replace(/\/$/, "")}${url}`;
}

/** OpenStreetMap link of the position (directions on any device). */
export function dealerMapHref(position: LatLng): string {
  const lat = position.lat.toFixed(6);
  const lng = position.lng.toFixed(6);
  return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=17/${lat}/${lng}`;
}

export function googleProfileHref(
  showcase: PublicDealerShowcase | undefined,
): string | null {
  if (!showcase?.google_place_id) return null;
  return `https://www.google.com/maps/search/?api=1&query=Google&query_place_id=${encodeURIComponent(
    showcase.google_place_id,
  )}`;
}

export function dealerSeoTitle(dealer: PublicDealer): string {
  const city = dealer.city?.trim();
  return city
    ? `${dealer.name} — ${city} PPF / boya koruma filmi`
    : `${dealer.name} — PPF / boya koruma filmi`;
}

export function dealerOgImage(dealer: PublicDealer): string | null {
  const firstPhoto = dealer.showcase?.photos?.[0]?.url;
  if (firstPhoto) return dealerPhotoSrc(firstPhoto);
  return dealerLogoSrc(dealer);
}

export function dealerCanonicalPath(dealer: PublicDealer): string {
  return `/bayi/${encodeURIComponent(dealer.code)}`;
}

export function openingHoursJsonLd(
  showcase: PublicDealerShowcase | undefined,
): string[] | undefined {
  if (!showcase?.working_hours?.length) return undefined;
  const dayMap: Record<string, string> = {
    monday: "Monday",
    tuesday: "Tuesday",
    wednesday: "Wednesday",
    thursday: "Thursday",
    friday: "Friday",
    saturday: "Saturday",
    sunday: "Sunday",
  };
  const rows = showcase.working_hours.flatMap((day) =>
    day.windows.map((w) => `${dayMap[day.day]} ${w.start}-${w.end}`),
  );
  return rows.length ? rows : undefined;
}

export function dealerJsonLd(dealer: PublicDealer): Record<string, unknown> {
  const position = dealerPosition(dealer);
  const showcase = dealer.showcase;
  const data: Record<string, unknown> = {
    "@context": "https://schema.org",
    "@type": ["AutoRepair", "LocalBusiness"],
    name: dealer.name,
    url: dealerCanonicalPath(dealer),
    address: {
      "@type": "PostalAddress",
      streetAddress: dealer.address || undefined,
      addressLocality: dealer.district || dealer.city || undefined,
      addressRegion: dealer.city || undefined,
    },
  };
  const logo = dealerLogoSrc(dealer);
  if (logo) data.image = logo;
  if (position) {
    data.geo = {
      "@type": "GeoCoordinates",
      latitude: position.lat,
      longitude: position.lng,
    };
  }
  const openingHours = openingHoursJsonLd(showcase);
  if (openingHours) data.openingHours = openingHours;
  if (
    showcase?.google_rating_source === "places" &&
    typeof showcase.google_rating === "number" &&
    typeof showcase.google_review_count === "number"
  ) {
    data.aggregateRating = {
      "@type": "AggregateRating",
      ratingValue: showcase.google_rating,
      reviewCount: showcase.google_review_count,
    };
  }
  return data;
}
