import {
  proxyPublicImage,
  vehicleHeroUpstreamPath,
} from "@/lib/server/public-image-proxy";

/**
 * Vehicle hero images (TEC-150): `/vehicle-heroes/default`,
 * `/vehicle-heroes/brands/{uuid}`, `/vehicle-heroes/models/{uuid}` → Go
 * `/v1/public/vehicle-heroes/*`. ETag / Cache-Control / 304 pass through.
 */
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ path: string[] }> };

async function handle(request: Request, { params }: Context) {
  const { path } = await params;
  return proxyPublicImage(request, vehicleHeroUpstreamPath(path));
}

export const GET = handle;
export const HEAD = handle;
