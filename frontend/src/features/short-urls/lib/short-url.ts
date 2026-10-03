/**
 * Short URLs (TEC-249): `/s/{token}` resolves through Go
 * `GET /v1/public/short-urls/{token}` and redirects to an internal path.
 * Not to be confused with `/share/s/{token}` (signed storage links).
 */

/** Stored token shape: case-sensitive base62, 4..16 (old hub tokens are 8). */
export const SHORT_URL_TOKEN_RE = /^[A-Za-z0-9]{4,16}$/;

/** Frontend areas a short URL may point at (same list as the Go usecase). */
export const SHORT_URL_ALLOWED_PREFIXES = ["/portal", "/garanti", "/bayi"];

/** Page shown for an expired, unknown or failed short link. */
export const SHORT_URL_UNAVAILABLE_PATH = "/link-unavailable";

export type ShortUrlUnavailableReason = "expired" | "not_found" | "error";

/**
 * Defence in depth: the redirect target must be a same-origin path under
 * an allowed prefix, even if the API returned something else.
 */
export function isAllowedShortUrlTarget(target: unknown): target is string {
  if (typeof target !== "string" || target.length > 2048) return false;
  if (!target.startsWith("/") || target.startsWith("//")) return false;
  if (/[\\\s\u0000-\u001f\u007f]/.test(target)) return false;
  const path = target.split(/[?#]/, 1)[0]!;
  const dotSegment = (seg: string) => {
    const s = seg.toLowerCase().replaceAll("%2e", ".");
    return s === "." || s === "..";
  };
  if (path.split("/").some(dotSegment)) return false;
  return SHORT_URL_ALLOWED_PREFIXES.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`),
  );
}

/** Normalizes the `?reason=` of the unavailable page. */
export function parseUnavailableReason(
  raw: string | string[] | undefined,
): ShortUrlUnavailableReason {
  const value = Array.isArray(raw) ? raw[0] : raw;
  return value === "expired" || value === "not_found" ? value : "error";
}
