import {
  brandLogoUpstreamPath,
  proxyPublicImage,
} from "@/lib/server/public-image-proxy";

/**
 * Fixed, cacheable car brand logo URL (TEC-150): `/brand-logos/{uuid}` →
 * Go `/v1/public/brand-logos/{uuid}`. ETag / Cache-Control / 304 pass through.
 */
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ uuid: string }> };

async function handle(request: Request, { params }: Context) {
  const { uuid } = await params;
  return proxyPublicImage(request, brandLogoUpstreamPath(uuid));
}

export const GET = handle;
export const HEAD = handle;
