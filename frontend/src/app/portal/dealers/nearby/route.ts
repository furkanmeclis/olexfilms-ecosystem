import {
  nearbyQueryString,
  parseNearbyQuery,
} from "@/features/dealers/lib/dealers";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Dealer finder search (TEC-242): `/portal/dealers/nearby?lat&lng&radius_km`
 * → Go `GET /v1/public/dealers/nearby` (TEC-240). No session and no cookies;
 * the client IP (Go's per-IP limit) and host (brand, K3) are forwarded.
 * A malformed query is 400 without calling Go; Go's status, body and
 * Retry-After are passed through.
 */
export const dynamic = "force-dynamic";

const JSON_HEADERS = {
  "Content-Type": "application/json",
  "Cache-Control": "no-store",
  "X-Content-Type-Options": "nosniff",
};

export async function GET(request: Request) {
  const query = parseNearbyQuery(new URL(request.url).searchParams);
  if (!query) {
    return new Response(
      JSON.stringify({
        success: false,
        error: { code: "VALIDATION_ERROR", message: "invalid lat/lng/radius" },
      }),
      { status: 400, headers: JSON_HEADERS },
    );
  }

  const headers = new Headers({ Accept: "application/json" });
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  const host = forwardedHostFromHeaders(request.headers);
  if (host) headers.set("X-Forwarded-Host", host);

  let res;
  try {
    res = await fetchUpstream(
      `public/dealers/nearby?${nearbyQueryString(query)}`,
      { method: "GET", headers },
    );
  } catch {
    return new Response(
      JSON.stringify({
        success: false,
        error: { code: "UPSTREAM_UNAVAILABLE", message: "unavailable" },
      }),
      { status: 502, headers: JSON_HEADERS },
    );
  }

  const out = new Headers(JSON_HEADERS);
  const retryAfter = res.headers.get("retry-after");
  if (retryAfter) out.set("Retry-After", retryAfter);
  return new Response(res.body, { status: res.status, headers: out });
}
