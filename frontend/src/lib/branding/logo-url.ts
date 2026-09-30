/**
 * Maps API `logo_url` (`/v1/...`) to same-origin BFF (`/api/v1/...`) for `<img>` / Avatar.
 * Auth cookies are sent automatically on same-origin requests (no presign).
 */
export function resolveLogoSrc(
  logoUrl: string | null | undefined,
  cacheKey?: string | number,
): string | undefined {
  if (!logoUrl) return undefined;

  let src = logoUrl;
  if (src.startsWith("/v1/")) {
    src = `/api${src}`;
  }

  if (cacheKey != null && cacheKey !== "") {
    const sep = src.includes("?") ? "&" : "?";
    src = `${src}${sep}v=${encodeURIComponent(String(cacheKey))}`;
  }

  return src;
}
