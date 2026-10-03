import { mapConfig, type LatLng } from "@/config/map";
import type { components } from "@/generated/api";

/**
 * Dealer finder (TEC-242): `/portal/dealers` asks the browser for its
 * position, then lists the brand's dealers near it through the same-origin
 * route `/portal/dealers/nearby` → Go `GET /v1/public/dealers/nearby`
 * (TEC-240).
 */
export type NearbyDealer = components["schemas"]["NearbyDealer"];

/** Search radius choices in km (Go accepts 0 < r ≤ 1000). */
export const RADIUS_OPTIONS = [25, 50, 100, 250, 500, 1000] as const;
export type RadiusKm = (typeof RADIUS_OPTIONS)[number];
/** Radius around the user's own position. */
export const LOCATED_RADIUS_KM: RadiusKm = 100;
/** Radius around the default centre: covers the whole country. */
export const FALLBACK_RADIUS_KM: RadiusKm = 1000;

/** Same-origin route of the nearby search (proxied to Go). */
export const NEARBY_ROUTE = "/portal/dealers/nearby";

/**
 * Distance in km in the page language: one decimal under 10 km, whole
 * kilometres above ("3,5 km", "128 km").
 */
export function formatDistanceKm(km: number, locale: string): string {
  const value = Number.isFinite(km) && km > 0 ? km : 0;
  const options: Intl.NumberFormatOptions = {
    style: "unit",
    unit: "kilometer",
    unitDisplay: "short",
    maximumFractionDigits: value < 10 ? 1 : 0,
  };
  try {
    return new Intl.NumberFormat(locale.replace("_", "-"), options).format(
      value,
    );
  } catch {
    return new Intl.NumberFormat("en", options).format(value);
  }
}

const E164_RE = /^\+[1-9]\d{6,14}$/;

/**
 * wa.me chat link of an E.164 number ("+905321234567" →
 * "https://wa.me/905321234567"). wa.me wants the digits only, without "+"
 * or separators. Anything that is not E.164 gives null (no button).
 */
export function whatsappHref(
  phone: string | null | undefined,
  text?: string,
): string | null {
  if (!phone) return null;
  const compact = phone.replace(/[\s\-().]/g, "");
  if (!E164_RE.test(compact)) return null;
  const base = `https://wa.me/${compact.slice(1)}`;
  return text ? `${base}?text=${encodeURIComponent(text)}` : base;
}

export type PositionResult =
  | { kind: "located"; center: LatLng }
  | {
      kind: "fallback";
      center: LatLng;
      reason: "denied" | "unavailable" | "unsupported";
    };

/** Minimal slice of `navigator.geolocation` (injectable in tests). */
export type GeolocationLike = Pick<Geolocation, "getCurrentPosition">;

/**
 * Asks the browser for its position. Denied, failed, timed out or missing
 * geolocation all resolve to the default centre with the reason, so the
 * page can show the map and a notice instead of hanging.
 */
export function locateUser(
  geolocation: GeolocationLike | null | undefined,
  timeoutMs = 10_000,
): Promise<PositionResult> {
  const fallback = (
    reason: "denied" | "unavailable" | "unsupported",
  ): PositionResult => ({
    kind: "fallback",
    center: { ...mapConfig.defaultCenter },
    reason,
  });
  if (!geolocation) return Promise.resolve(fallback("unsupported"));
  return new Promise((resolve) => {
    try {
      geolocation.getCurrentPosition(
        (pos) =>
          resolve({
            kind: "located",
            center: { lat: pos.coords.latitude, lng: pos.coords.longitude },
          }),
        (err) =>
          // PERMISSION_DENIED is 1 (also when a Permissions-Policy blocks it).
          resolve(fallback(err?.code === 1 ? "denied" : "unavailable")),
        { enableHighAccuracy: false, timeout: timeoutMs, maximumAge: 300_000 },
      );
    } catch {
      resolve(fallback("unavailable"));
    }
  });
}

export type NearbyQuery = { lat: number; lng: number; radiusKm: number };

function finiteIn(raw: string | null, min: number, max: number) {
  if (raw === null || raw.trim() === "") return null;
  const n = Number(raw);
  return Number.isFinite(n) && n >= min && n <= max ? n : null;
}

/**
 * Validates the route's query (`lat`, `lng`, optional `radius_km`) with the
 * same bounds as Go; null means 400 without calling Go.
 */
export function parseNearbyQuery(params: URLSearchParams): NearbyQuery | null {
  const lat = finiteIn(params.get("lat"), -90, 90);
  const lng = finiteIn(params.get("lng"), -180, 180);
  if (lat === null || lng === null) return null;
  const rawRadius = params.get("radius_km");
  let radiusKm = LOCATED_RADIUS_KM as number;
  if (rawRadius !== null) {
    const r = finiteIn(rawRadius, 0, 1000);
    if (r === null || r <= 0) return null;
    radiusKm = r;
  }
  return { lat, lng, radiusKm };
}

export function nearbyQueryString(q: NearbyQuery): string {
  return new URLSearchParams({
    lat: q.lat.toFixed(6),
    lng: q.lng.toFixed(6),
    radius_km: String(q.radiusKm),
  }).toString();
}

export type NearbyResult =
  | { kind: "ok"; dealers: NearbyDealer[] }
  | { kind: "rate_limited" }
  | { kind: "error" };

/** Browser-side call of the same-origin nearby route. */
export async function fetchNearbyDealers(
  q: NearbyQuery,
  fetchImpl: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<NearbyResult> {
  let res: Response;
  try {
    res = await fetchImpl(`${NEARBY_ROUTE}?${nearbyQueryString(q)}`, {
      headers: { Accept: "application/json" },
      signal,
    });
  } catch {
    return { kind: "error" };
  }
  if (res.status === 429) return { kind: "rate_limited" };
  if (!res.ok) return { kind: "error" };
  try {
    const body = (await res.json()) as {
      data?: { items?: NearbyDealer[] };
    };
    const items = body.data?.items;
    return Array.isArray(items)
      ? { kind: "ok", dealers: items }
      : { kind: "error" };
  } catch {
    return { kind: "error" };
  }
}
