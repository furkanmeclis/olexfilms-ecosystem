import type { Metadata } from "next";
import { cookies, headers } from "next/headers";

import { i18nConfig, normalizeLocale } from "@/config/i18n";
import { routes } from "@/config/routes";
import { site } from "@/config/site";
import { LandingView } from "@/features/landing/components/landing-view";
import { fetchDealerApplicationConfig } from "@/features/public-leads/lib/dealer-application";
import { loadMessages, translate } from "@/lib/i18n/messages";
import { resolveRequestLocale } from "@/lib/i18n/request-locale";
import { localeAlternates } from "@/lib/seo/sitemap";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Public landing page (TEC-247). Language: `?lang=`, else the language
 * cookie, else the browser's Accept-Language (same order as /garanti).
 */
export const dynamic = "force-dynamic";

type PageProps = {
  searchParams: Promise<{ lang?: string | string[] }>;
};

async function pageLocale(searchParams: PageProps["searchParams"]) {
  const { lang } = await searchParams;
  const store = await cookies();
  const requestHeaders = await headers();
  const resolved = resolveRequestLocale(
    (name) => store.get(name)?.value,
    requestHeaders.get("accept-language"),
  );
  const fromQuery = normalizeLocale(Array.isArray(lang) ? lang[0] : lang);
  return { locale: fromQuery ?? resolved.locale, fromQuery };
}

export async function generateMetadata({
  searchParams,
}: PageProps): Promise<Metadata> {
  const { locale } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  const title = translate(locale, "landing.meta.title");
  const description = translate(locale, "landing.meta.description");
  const url = `${site.url}${routes.public.root}`;
  return {
    title: { absolute: title },
    description,
    alternates: {
      canonical: routes.public.root,
      languages: localeAlternates(url),
    },
    openGraph: {
      type: "website",
      siteName: site.name,
      title,
      description,
      locale,
      url: routes.public.root,
      images: [{ ...site.ogImage, alt: title }],
    },
    twitter: {
      card: "summary_large_image",
      title,
      description,
      images: [site.ogImage.url],
    },
  };
}

export default async function PublicHomePage({ searchParams }: PageProps) {
  const { locale, fromQuery } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  const requestHeaders = await headers();
  // TEC-320: the "become a dealer" link shows only while the form is open.
  const application = await fetchDealerApplicationConfig(
    {
      clientIp: clientIpFromHeaders(requestHeaders),
      forwardedHost: forwardedHostFromHeaders(requestHeaders),
    },
    fetchUpstream,
  );
  return (
    <LandingView
      locale={locale}
      lang={fromQuery ?? undefined}
      dealerApplicationOpen={application === "open"}
    />
  );
}
