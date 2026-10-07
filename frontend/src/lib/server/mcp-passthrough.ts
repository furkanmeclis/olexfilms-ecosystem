import { upstreamConfig } from "@/config/api";
import {
  clientIpFromHeaders,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * MCP / OAuth passthrough (TEC-400): `/oauth/*`, `/.well-known/*` and
 * `/mcp/*` → the same paths on the Go API (outside `/v1`). In production
 * only the frontend is public, so MCP clients reach the authorization server
 * and the MCP endpoints through here.
 *
 * Unlike the panel / portal BFFs this proxy holds no session and does no
 * CSRF check: requests carry their own Bearer token or hit public OAuth
 * endpoints. Cookies are never forwarded (a browser on the app domain sends
 * them to /oauth/authorize; Go must not see them). Response bodies stream
 * through unbuffered (MCP Streamable HTTP / SSE).
 */

/** Request headers forwarded to Go. */
const FORWARD_REQUEST_HEADERS = [
  "accept",
  "accept-language",
  "authorization",
  "content-type",
  "last-event-id",
  "mcp-protocol-version",
  "mcp-session-id",
  "user-agent",
  "x-request-id",
] as const;

/** Response headers returned to the client. */
const FORWARD_RESPONSE_HEADERS = [
  "access-control-allow-headers",
  "access-control-allow-methods",
  "access-control-allow-origin",
  "access-control-expose-headers",
  "access-control-max-age",
  "cache-control",
  "content-type",
  "location",
  "mcp-session-id",
  "pragma",
  "retry-after",
  "www-authenticate",
  "x-request-id",
] as const;

/** Go API origin: the upstream base without the trailing `/v1`. */
export function apiOrigin(): string {
  return upstreamConfig.baseUrl.replace(/\/v1\/?$/i, "");
}

/** Rejects segments that could escape the mounted prefix once normalised. */
export function isUnsafePath(segments: string[]): boolean {
  if (segments.length === 0) return true;
  return segments.some(
    (seg) => seg === "" || seg === "." || seg === ".." || /[/\\%?#]/.test(seg),
  );
}

/** Builds the upstream request headers (exported for tests). */
export function mcpUpstreamHeaders(request: Request): Headers {
  const out = new Headers();
  for (const key of FORWARD_REQUEST_HEADERS) {
    const value = request.headers.get(key);
    if (value) out.set(key, value);
  }
  // Go's per-IP OAuth limits key on the client IP; brand from host.
  const clientIp = clientIpFromHeaders(request.headers);
  if (clientIp) out.set("X-Forwarded-For", clientIp);
  const forwardedHost = forwardedHostFromHeaders(request.headers);
  if (forwardedHost) out.set("X-Forwarded-Host", forwardedHost);
  return out;
}

function jsonError(status: number, error: string, description: string) {
  return new Response(
    JSON.stringify({ error, error_description: description }),
    {
      status,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-store",
      },
    },
  );
}

/**
 * Forwards the request to `{prefix}/{segments}` on Go and streams the answer
 * back. `prefix` is `/oauth`, `/.well-known` or `/mcp`.
 */
export async function mcpPassthrough(
  request: Request,
  prefix: "/oauth" | "/.well-known" | "/mcp",
  segments: string[],
): Promise<Response> {
  if (isUnsafePath(segments)) {
    return jsonError(404, "not_found", "Not found");
  }
  const url = new URL(request.url);
  const target = `${apiOrigin()}${prefix}/${segments.join("/")}${url.search}`;
  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  const body = hasBody ? await request.arrayBuffer() : undefined;

  let upstream: Response;
  try {
    upstream = await fetch(target, {
      method: request.method,
      headers: mcpUpstreamHeaders(request),
      body: body && body.byteLength > 0 ? body : undefined,
      redirect: "manual",
      cache: "no-store",
      signal: request.signal,
    });
  } catch {
    return jsonError(
      503,
      "temporarily_unavailable",
      "The service is temporarily unavailable. Try again shortly.",
    );
  }

  const headers = new Headers();
  for (const key of FORWARD_RESPONSE_HEADERS) {
    const value = upstream.headers.get(key);
    if (value) headers.set(key, value);
  }
  const noBody =
    upstream.status === 204 ||
    upstream.status === 304 ||
    request.method === "HEAD";
  return new Response(noBody ? null : upstream.body, {
    status: upstream.status,
    headers,
  });
}
