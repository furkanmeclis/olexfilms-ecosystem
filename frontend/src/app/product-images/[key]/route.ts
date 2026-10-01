import { fetchUpstreamStream } from "@/lib/server/upstream";

/**
 * Public product image (TEC-152): `/product-images/{key}` → Go
 * `/v1/public/product-images/{key}`. No session, no cookies. A Route Handler
 * instead of a `next.config` rewrite because rewrites are fixed at build time
 * while `API_URL` is a runtime setting of the standalone image.
 */
const KEY = /^[0-9a-f]{32}\.(jpg|png|webp)$/;

const FORWARDED = [
  "content-type",
  "content-length",
  "cache-control",
  "etag",
  "x-content-type-options",
] as const;

async function handle(
  request: Request,
  { params }: { params: Promise<{ key: string }> },
): Promise<Response> {
  const { key } = await params;
  if (!KEY.test(key)) {
    return new Response(null, { status: 404 });
  }
  const headers = new Headers({ Accept: "image/*" });
  const ifNoneMatch = request.headers.get("if-none-match");
  if (ifNoneMatch) headers.set("If-None-Match", ifNoneMatch);
  let upstream;
  try {
    upstream = await fetchUpstreamStream(`public/product-images/${key}`, {
      method: request.method === "HEAD" ? "HEAD" : "GET",
      headers,
      signal: request.signal,
    });
  } catch {
    return new Response(null, {
      status: 503,
      headers: { "Retry-After": "5", "Cache-Control": "no-store" },
    });
  }
  const out = new Headers();
  for (const name of FORWARDED) {
    const value = upstream.headers.get(name);
    if (value) out.set(name, value);
  }
  if (upstream.status !== 200 && upstream.status !== 304) {
    await upstream.body?.cancel().catch(() => undefined);
    return new Response(null, {
      status: upstream.status === 404 ? 404 : 502,
      headers: { "Cache-Control": "no-store" },
    });
  }
  return new Response(upstream.status === 304 ? null : upstream.body, {
    status: upstream.status,
    headers: out,
  });
}

export const GET = handle;
export const HEAD = handle;
