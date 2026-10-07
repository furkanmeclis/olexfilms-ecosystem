import type { MetadataRoute } from "next";

import { SUPPORTED_LOCALES } from "@/config/i18n";
import { routes } from "@/config/routes";
import type { PublicDealerCode } from "@/features/dealers/lib/dealer-sitemap";

/**
 * hreflang alternates of a public page (TEC-251). Public pages pick their
 * language from `?lang=` (landing, /bayi, /garanti, /bayi-basvuru), so each of the 13
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

export type SitemapOptions = {
  /** TEC-320: `/bayi-basvuru` is listed only while the form is open. */
  dealerApplication?: boolean;
};

/**
 * Sitemap entries: the landing page, the dealer application form while it
 * is open, and each active dealer's `/bayi/{code}` showcase. Panel, portal,
 * quote links and other signed-in or token areas are never listed.
 */
export function buildSitemap(
  siteUrl: string,
  dealers: readonly PublicDealerCode[],
  now: Date = new Date(),
  { dealerApplication = false }: SitemapOptions = {},
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
  if (dealerApplication) {
    const url = `${base}${routes.public.dealerApplication}`;
    entries.push({
      url,
      lastModified: now,
      changeFrequency: "monthly",
      priority: 0.8,
      alternates: { languages: localeAlternates(url) },
    });
  }
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
