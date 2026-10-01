import { fetchUpstreamStream } from "@/lib/server/upstream";

/**
 * Fixed public image URLs of the vehicle catalog (TEC-150):
 *   /brand-logos/{uuid}                → Go /v1/public/brand-logos/{uuid}
 *   /vehicle-heroes/default            → Go /v1/public/vehicle-heroes/default
 *   /vehicle-heroes/{brands|models}/{uuid}
 *
 * A route handler (not a next.config rewrite) because the Go base URL is the
 * runtime `API_URL`, while rewrites are frozen at build time. No auth, no
 * cookies: the upstream answers ETag + Cache-Control and a matching
 * If-None-Match gets 304; both pass through unchanged so browsers and CDNs
 * cache the image.
 */

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Response headers copied from Go (everything else is dropped). */
const PASS_HEADERS = [
  "content-type",
  "content-length",
  "cache-control",
  "etag",
  "last-modified",
  "x-content-type-options",
  "content-security-policy",
] as const;

/** Upstream path for a brand logo, or null for a malformed id. */
export function brandLogoUpstreamPath(uuid: string): string | null {
  return UUID_RE.test(uuid) ? `public/brand-logos/${uuid.toLowerCase()}` : null;
}

/** Uploaded product image key (TEC-152): 32 hex + raster extension. */
const PRODUCT_IMAGE_KEY_RE = /^[0-9a-f]{32}\.(jpg|png|webp)$/;

/** Upstream path for a product image key, or null for a malformed key. */
export function productImageUpstreamPath(key: string): string | null {
  return PRODUCT_IMAGE_KEY_RE.test(key) ? `public/product-images/${key}` : null;
}

/** Upstream path for a hero URL's segments, or null when not allowed. */
export function vehicleHeroUpstreamPath(segments: string[]): string | null {
  if (segments.length === 1 && segments[0] === "default") {
    return "public/vehicle-heroes/default";
  }
  if (
    segments.length === 2 &&
    (segments[0] === "brands" || segments[0] === "models") &&
    UUID_RE.test(segments[1]!)
  ) {
    return `public/vehicle-heroes/${segments[0]}/${segments[1]!.toLowerCase()}`;
  }
  return null;
}

/** Picks the cache-relevant headers of an upstream image response. */
export function publicImageHeaders(upstream: Headers): Headers {
  const out = new Headers();
  for (const name of PASS_HEADERS) {
    const value = upstream.get(name);
    if (value) out.set(name, value);
  }
  return out;
}

function notFound() {
  return new Response(null, {
    status: 404,
    headers: { "cache-control": "no-store" },
  });
}

/** Streams a public image from Go, keeping its cache headers. */
export async function proxyPublicImage(
  request: Request,
  upstreamPath: string | null,
): Promise<Response> {
  if (!upstreamPath) return notFound();
  const headers = new Headers({ Accept: "image/*" });
  const ifNoneMatch = request.headers.get("if-none-match");
  if (ifNoneMatch) headers.set("If-None-Match", ifNoneMatch);

  let upstream;
  try {
    upstream = await fetchUpstreamStream(upstreamPath, {
      method: request.method === "HEAD" ? "HEAD" : "GET",
      headers,
      signal: request.signal,
    });
  } catch {
    return new Response(null, {
      status: 502,
      headers: { "cache-control": "no-store" },
    });
  }

  const out = publicImageHeaders(upstream.headers);
  if (upstream.status === 304) {
    out.delete("content-length");
    out.delete("content-type");
    return new Response(null, { status: 304, headers: out });
  }
  if (upstream.status !== 200) {
    await upstream.body?.cancel().catch(() => undefined);
    return upstream.status === 404 || upstream.status === 400
      ? notFound()
      : new Response(null, {
          status: 502,
          headers: { "cache-control": "no-store" },
        });
  }
  return new Response(request.method === "HEAD" ? null : upstream.body, {
    status: 200,
    headers: out,
  });
}
