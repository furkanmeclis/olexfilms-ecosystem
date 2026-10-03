import type { MetadataRoute } from "next";

import { SUPPORTED_LOCALES } from "@/config/i18n";
import { routes } from "@/config/routes";
import type { PublicDealerCode } from "@/features/dealers/lib/dealer-sitemap";

/**
 * hreflang alternates of a public page (TEC-251). Public pages pick their
 * language from `?lang=` (landing, /bayi, /garanti), so each of the 13
 * languages is the same URL with that parameter; x-default is the plain URL.
 */
export function localeAlternates(url: string): Record<string, string> {
  const languages: Record<string, string> = {};
  for (const locale of SUPPORTED_LOCALES) {
    const target = new URL(url);
    target.searchParams.set("lang", locale);
    languages[locale] = target.toString();
  }
  languages["x-default"] = url;
  return languages;
}

/**
 * Sitemap entries: the landing page and each active dealer's `/bayi/{code}`
 * showcase. Panel, portal and other signed-in areas are never listed.
 */
export function buildSitemap(
  siteUrl: string,
  dealers: readonly PublicDealerCode[],
  now: Date = new Date(),
): MetadataRoute.Sitemap {
  const base = siteUrl.replace(/\/$/, "");
  const landing = `${base}${routes.public.root}`;
  const entries: MetadataRoute.Sitemap = [
    {
      url: landing,
      lastModified: now,
      changeFrequency: "monthly",
      priority: 1,
      alternates: { languages: localeAlternates(landing) },
    },
  ];
  for (const dealer of dealers) {
    const url = `${base}${routes.public.dealer(dealer.code)}`;
    const updated = new Date(dealer.updated_at);
    entries.push({
      url,
      lastModified: Number.isNaN(updated.getTime()) ? now : updated,
      changeFrequency: "weekly",
      priority: 0.7,
      alternates: { languages: localeAlternates(url) },
    });
  }
  return entries;
}
