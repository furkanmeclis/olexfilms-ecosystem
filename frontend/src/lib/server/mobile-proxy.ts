import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Mobile API passthrough (TEC-91): `/api/v1/mobile/*` → Go `/v1/mobile/*`.
 *
 * Unlike the panel / portal BFFs this proxy holds no session: the mobile app
 * keeps its own token pair and sends `Authorization: Bearer`. The header is
 * forwarded as is; no cookie is read or written, tokens are not stripped from
 * responses and there is no CSRF check (no ambient credential to abuse).
 *
 * A request that carries a Cookie header is refused with 400, so cookie and
 * Bearer authentication never mix on this path (a browser on the app domain
 * always sends its cookies, a mobile client never has any).
 */

/** Request headers the mobile app may send upstream. */
const FORWARD_REQUEST_HEADERS = [
  "accept",
  "accept-language",
  "authorization",
  "content-type",
  "idempotency-key",
  "user-agent",
  "x-app-version",
  "x-mobile-api-version",
  "x-request-id",
] as const;

/** Response headers returned to the app. */
const FORWARD_RESPONSE_HEADERS = [
  "content-type",
  "content-disposition",
  "cache-control",
  "retry-after",
  "x-request-id",
  "x-mobile-api-version",
] as const;

/** Rejects segments that could escape `/v1/mobile/*` once normalised. */
function isUnsafePath(segments: string[]): boolean {
  if (segments.length === 0) return true;
  return segments.some(
    (seg) => seg === "" || seg === "." || seg === ".." || /[/\\%?#]/.test(seg),
  );
}

function jsonError(status: number, code: string, message: string): Response {
  return new Response(
    JSON.stringify({ success: false, error: { code, message } }),
    {
      status,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-store",
      },
    },
  );
}

/** Builds the upstream request headers (exported for tests). */
export function mobileUpstreamHeaders(request: Request): Headers {
  const out = new Headers();
  for (const key of FORWARD_REQUEST_HEADERS) {
    const value = request.headers.get(key);
    if (value) out.set(key, value);
  }
  if (!out.has("accept")) out.set("accept", "application/json");
  // Go rate limits and session rows key on the client IP; brand from host.
  const clientIp = clientIpFromHeaders(request.headers);
  if (clientIp) out.set("X-Forwarded-For", clientIp);
  const forwardedHost = forwardedHostFromHeaders(request.headers);
  if (forwardedHost) out.set("X-Forwarded-Host", forwardedHost);
  return out;
}

export async function proxyMobileToUpstream(
  pathSegments: string[],
  request: Request,
): Promise<Response> {
  if (isUnsafePath(pathSegments)) {
    return jsonError(404, "NOT_FOUND", "Not found");
  }
  if (request.headers.has("cookie")) {
    return jsonError(
      400,
      "COOKIE_NOT_ALLOWED",
      "The mobile API authenticates with a Bearer token only; send no cookies.",
    );
  }
  const url = new URL(request.url);
  const path = `mobile/${pathSegments.join("/")}${url.search}`;
  const body =
    request.method === "GET" || request.method === "HEAD"
      ? null
      : await request.arrayBuffer();

  let result;
  try {
    result = await fetchUpstream(path, {
      method: request.method,
      headers: mobileUpstreamHeaders(request),
      body: body && body.byteLength > 0 ? body : null,
    });
  } catch {
    return jsonError(
      503,
      "SERVICE_UNAVAILABLE",
      "The service is temporarily unavailable. Try again shortly.",
    );
  }

  const headers = new Headers();
  for (const key of FORWARD_RESPONSE_HEADERS) {
    const value = result.headers.get(key);
    if (value) headers.set(key, value);
  }
  return new Response(result.body, { status: result.status, headers });
}
