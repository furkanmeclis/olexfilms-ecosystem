import type { MetadataRoute } from "next";

import { site } from "@/config/site";
import { fetchPublicDealerCodes } from "@/features/dealers/lib/dealer-sitemap";
import { fetchDealerApplicationConfig } from "@/features/public-leads/lib/dealer-application";
import { fetchUpstream } from "@/lib/server/upstream";
import { buildSitemap } from "@/lib/seo/sitemap";

/**
 * TEC-251: landing + active dealers' `/bayi/{code}` pages with hreflang
 * alternates; TEC-320: `/bayi-basvuru` only while the form is open.
 * Rendered per request (Go is not reachable at build time);
 * the dealer list is read for the configured site host, so URLs and brand
 * always match `site.url` whatever Host the request carries.
 */
export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const host = new URL(site.url).host;
  const [dealers, application] = await Promise.all([
    fetchPublicDealerCodes(host, fetchUpstream),
    fetchDealerApplicationConfig(
      { clientIp: null, forwardedHost: host },
      fetchUpstream,
    ),
  ]);
  return buildSitemap(site.url, dealers, new Date(), {
    dealerApplication: application === "open",
  });
}
