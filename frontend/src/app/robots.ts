import type { MetadataRoute } from "next";

import { site } from "@/config/site";
import { ROBOTS_DISALLOW } from "@/lib/seo/robots";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: ["/"],
        disallow: [...ROBOTS_DISALLOW],
      },
    ],
    sitemap: `${site.url}/sitemap.xml`,
    host: site.url,
  };
}
