import { upstreamConfig } from "@/config/api";

export type UpstreamResult = {
  status: number;
  headers: Headers;
  body: ArrayBuffer;
};

export type UpstreamStreamResult = {
  status: number;
  headers: Headers;
  body: ReadableStream<Uint8Array> | null;
};

function buildUpstreamUrl(pathWithQuery: string): string {
  return `${upstreamConfig.baseUrl.replace(/\/$/, "")}/${pathWithQuery.replace(/^\//, "")}`;
}

/**
 * Best-effort client IP for the Go API's per-IP rate limits and audit trail.
 * Prefer the edge proxy's X-Real-IP, else the right-most X-Forwarded-For hop
 * (appended by our own proxy; left-most entries are client-controlled).
 * Returns null when the request carries neither.
 */
export function clientIpFromHeaders(headers: Headers): string | null {
  const real = headers.get("x-real-ip")?.trim();
  if (real) return real;
  const xff = headers.get("x-forwarded-for");
  if (!xff) return null;
  const hops = xff
    .split(",")
    .map((hop) => hop.trim())
    .filter(Boolean);
  return hops.length > 0 ? hops[hops.length - 1]! : null;
}

/**
 * Browser-facing host of the request, for brand resolution in Go (K3).
 * Prefers the proxy's X-Forwarded-Host (first hop), then Host.
 */
export function forwardedHostFromHeaders(headers: Headers): string | null {
  const raw = headers.get("x-forwarded-host") ?? headers.get("host");
  const first = raw?.split(",")[0]?.trim();
  return first ? first : null;
}

function scrubUpstreamHeaders(init?: HeadersInit): Headers {
  const headers = new Headers(init);
  headers.delete("host");
  headers.delete("connection");
  headers.delete("content-length");
  // Browser Origin/Cookie must not leak to Go — auth is Bearer only
  headers.delete("cookie");
  headers.delete("origin");
  return headers;
}

/**
 * Forward a request to the Go API and buffer the body.
 * Caller attaches Authorization when needed.
 */
export async function fetchUpstream(
  pathWithQuery: string,
  init: {
    method: string;
    headers?: HeadersInit;
    body?: BodyInit | null;
  },
): Promise<UpstreamResult> {
  const response = await fetch(buildUpstreamUrl(pathWithQuery), {
    method: init.method,
    headers: scrubUpstreamHeaders(init.headers),
    body:
      init.body && init.method !== "GET" && init.method !== "HEAD"
        ? init.body
        : undefined,
    cache: "no-store",
  });

  return {
    status: response.status,
    headers: response.headers,
    body: await response.arrayBuffer(),
  };
}

/**
 * Forward a request and keep the body as a live stream (SSE / long-poll).
 * Do not call arrayBuffer() — that would buffer and break event streams.
 */
export async function fetchUpstreamStream(
  pathWithQuery: string,
  init: {
    method: string;
    headers?: HeadersInit;
    body?: BodyInit | null;
    signal?: AbortSignal;
  },
): Promise<UpstreamStreamResult> {
  const response = await fetch(buildUpstreamUrl(pathWithQuery), {
    method: init.method,
    headers: scrubUpstreamHeaders(init.headers),
    body:
      init.body && init.method !== "GET" && init.method !== "HEAD"
        ? init.body
        : undefined,
    cache: "no-store",
    signal: init.signal,
  });

  return {
    status: response.status,
    headers: response.headers,
    body: response.body,
  };
}
