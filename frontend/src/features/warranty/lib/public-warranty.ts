import type { components } from "@/generated/api";

/**
 * Public warranty page (TEC-189): `/garanti/{public_code}` reads Go
 * `GET /v1/public/warranties/{public_code}` on the server. The response has
 * no personal data (masked plate, last 4 VIN characters only).
 */
export type PublicWarranty = components["schemas"]["PublicWarranty"];
export type PublicWarrantyStatus = PublicWarranty["status"];

/** Same shape as the Go check (chk_warranties_public_code). */
export const PUBLIC_CODE_RE = /^[A-Za-z0-9_-]{12,32}$/;

export type PublicWarrantyResult =
  | { kind: "ok"; warranty: PublicWarranty }
  | { kind: "not_found" }
  | { kind: "rate_limited"; retryAfter: number | null }
  | { kind: "error" };

/** Seconds of a Retry-After header (delta-seconds form), else null. */
export function parseRetryAfter(
  value: string | null | undefined,
): number | null {
  if (!value) return null;
  const n = Number.parseInt(value.trim(), 10);
  return Number.isFinite(n) && n > 0 ? n : null;
}

export type UpstreamResponse = {
  status: number;
  headers: Headers;
  body: ArrayBuffer;
};

export type UpstreamFetcher = (
  pathWithQuery: string,
  init: { method: string; headers?: HeadersInit },
) => Promise<UpstreamResponse>;

export type PublicWarrantyRequest = {
  /** Browser IP: the Go per-IP rate limit keys on it. */
  clientIp: string | null;
  /** Browser host: Go resolves the brand from it (K3, K20). */
  forwardedHost: string | null;
};

/**
 * Looks a public code up. A malformed code is not_found without a request
 * (Go answers the same 404); 429 keeps Retry-After; anything else unexpected
 * is error.
 */
export async function fetchPublicWarranty(
  code: string,
  request: PublicWarrantyRequest,
  fetcher: UpstreamFetcher,
): Promise<PublicWarrantyResult> {
  if (!PUBLIC_CODE_RE.test(code)) return { kind: "not_found" };
  const headers = new Headers({ Accept: "application/json" });
  if (request.clientIp) headers.set("X-Forwarded-For", request.clientIp);
  if (request.forwardedHost)
    headers.set("X-Forwarded-Host", request.forwardedHost);

  let res: UpstreamResponse;
  try {
    res = await fetcher(`public/warranties/${encodeURIComponent(code)}`, {
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
      data?: PublicWarranty;
    };
    if (!envelope.data?.public_code) return { kind: "error" };
    return { kind: "ok", warranty: envelope.data };
  } catch {
    return { kind: "error" };
  }
}

/** Badge tone of a warranty status. */
export function statusTone(
  status: PublicWarrantyStatus,
): "success" | "warning" | "danger" {
  if (status === "active") return "success";
  if (status === "expired") return "warning";
  return "danger";
}
