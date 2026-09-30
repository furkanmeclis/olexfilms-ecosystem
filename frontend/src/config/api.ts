/**
 * Browser always talks to the same-origin BFF.
 * OpenAPI paths are `/v1/...`; BFF mounts at `/api/v1/...` → base `/api`.
 * Upstream Go API URL is server-only (`API_URL`) — never expose tokens to the client.
 */
export const apiConfig = {
  /** Same-origin BFF base used by openapi-fetch in the browser */
  baseUrl: process.env.NEXT_PUBLIC_BFF_BASE_URL ?? "/api",
  timeoutMs: 30_000,
} as const;

/**
 * Server-only upstream (Route Handlers). Do not import from Client Components.
 * `API_URL` must be the Go `/v1` base (e.g. http://127.0.0.1:8080/v1).
 * Paths passed to fetchUpstream are relative to that base (`auth/login` → `/v1/auth/login`).
 */
export const upstreamConfig = {
  baseUrl: normalizeUpstreamBase(
    process.env.API_URL ??
      process.env.NEXT_PUBLIC_API_URL ??
      "http://127.0.0.1:8080/v1",
  ),
} as const;

/** Ensure host-only API_URL still hits Go `/v1` routes. */
function normalizeUpstreamBase(raw: string): string {
  const trimmed = raw.replace(/\/$/, "");
  if (/\/v1$/i.test(trimmed)) return trimmed;
  return `${trimmed}/v1`;
}
