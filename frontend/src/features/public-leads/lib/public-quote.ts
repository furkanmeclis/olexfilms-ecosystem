import {
  parseRetryAfter,
  type PublicWarrantyRequest,
  type UpstreamFetcher,
  type UpstreamResponse,
} from "@/features/warranty/lib/public-warranty";
import type { components } from "@/generated/api";

/**
 * Public quote page (TEC-320): `/teklif/{token}` reads Go
 * `GET /v1/public/quotes/{token}` on the server. The response carries the
 * issuing organization, the lines and the totals only, never the
 * recipient's phone or e-mail (TEC-315).
 */
export type PublicQuote = components["schemas"]["PublicQuote"];
export type PublicQuoteLine = PublicQuote["lines"][number];

/** Quote tokens are UUIDs; anything else is a 404 without a Go request. */
export const QUOTE_TOKEN_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export type PublicQuoteResult =
  | { kind: "ok"; quote: PublicQuote }
  | { kind: "not_found" }
  | { kind: "rate_limited"; retryAfter: number | null }
  | { kind: "error" };

/** Outcome of a PDF download that came back to the page (`?pdf=`). */
export type QuotePdfNotice = "pending" | "rate_limited" | "unavailable";

export function parseQuotePdfNotice(
  value: string | string[] | undefined,
): QuotePdfNotice | null {
  const v = Array.isArray(value) ? value[0] : value;
  return v === "pending" || v === "rate_limited" || v === "unavailable"
    ? v
    : null;
}

export function publicQuotePath(token: string): string {
  return `/teklif/${encodeURIComponent(token)}`;
}

/** Same-origin PDF link; the route handler streams Go's `/pdf/file`. */
export function publicQuotePdfHref(token: string, locale: string): string {
  const q = new URLSearchParams({ lang: locale });
  return `${publicQuotePath(token)}/pdf?${q.toString()}`;
}

/** Upstream headers of a public request: client IP (limit) and host (K3). */
export function publicUpstreamHeaders(
  request: PublicWarrantyRequest,
  accept = "application/json",
): Headers {
  const headers = new Headers({ Accept: accept });
  if (request.clientIp) headers.set("X-Forwarded-For", request.clientIp);
  if (request.forwardedHost)
    headers.set("X-Forwarded-Host", request.forwardedHost);
  return headers;
}

/**
 * Looks a quote token up. A malformed token is not_found without a request
 * (Go answers the same 404); 429 keeps Retry-After; anything else
 * unexpected is error.
 */
export async function fetchPublicQuote(
  token: string,
  request: PublicWarrantyRequest,
  fetcher: UpstreamFetcher,
): Promise<PublicQuoteResult> {
  if (!QUOTE_TOKEN_RE.test(token)) return { kind: "not_found" };
  let res: UpstreamResponse;
  try {
    res = await fetcher(`public/quotes/${encodeURIComponent(token)}`, {
      method: "GET",
      headers: publicUpstreamHeaders(request),
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
      data?: PublicQuote;
    };
    const quote = envelope.data;
    if (!quote?.uuid || !Array.isArray(quote.lines)) return { kind: "error" };
    return { kind: "ok", quote };
  } catch {
    return { kind: "error" };
  }
}

/** Decimal string of the API ("100.00") as a number for display. */
export function amount(value: string | null | undefined): number | null {
  if (value === null || value === undefined || value === "") return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

/** Whether the quote's validity date has passed. */
export function isQuoteExpired(quote: PublicQuote, now = new Date()): boolean {
  if (!quote.valid_until) return false;
  const until = new Date(quote.valid_until);
  return !Number.isNaN(until.getTime()) && until.getTime() < now.getTime();
}
