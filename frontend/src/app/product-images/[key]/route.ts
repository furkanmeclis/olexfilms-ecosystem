import {
  productImageUpstreamPath,
  proxyPublicImage,
} from "@/lib/server/public-image-proxy";

/**
 * Fixed, cacheable product image URL (TEC-152): `/product-images/{key}` →
 * Go `/v1/public/product-images/{key}`. Same proxy as the vehicle catalog
 * images (TEC-150): ETag / Cache-Control / 304 pass through, no cookies.
 */
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ key: string }> };

async function handle(request: Request, { params }: Context) {
  const { key } = await params;
  return proxyPublicImage(request, productImageUpstreamPath(key));
}

export const GET = handle;
export const HEAD = handle;
