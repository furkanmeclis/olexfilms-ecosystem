import type { MetadataRoute } from "next";

import { site } from "@/config/site";
import { fetchPublicDealerCodes } from "@/features/dealers/lib/dealer-sitemap";
import { fetchUpstream } from "@/lib/server/upstream";
import { buildSitemap } from "@/lib/seo/sitemap";

/**
 * TEC-251: landing + active dealers' `/bayi/{code}` pages with hreflang
 * alternates. Rendered per request (Go is not reachable at build time);
 * the dealer list is read for the configured site host, so URLs and brand
 * always match `site.url` whatever Host the request carries.
 */
export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const dealers = await fetchPublicDealerCodes(
    new URL(site.url).host,
    fetchUpstream,
  );
  return buildSitemap(site.url, dealers);
}
