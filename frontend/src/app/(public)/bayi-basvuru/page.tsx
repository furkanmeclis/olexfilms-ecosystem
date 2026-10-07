import type { Metadata } from "next";
import { headers } from "next/headers";
import { notFound } from "next/navigation";
import { cache } from "react";

import { routes } from "@/config/routes";
import { site } from "@/config/site";
import { DealerApplicationView } from "@/features/public-leads/components/dealer-application-view";
import { fetchDealerApplicationConfig } from "@/features/public-leads/lib/dealer-application";
import { publicPageLocale } from "@/features/public-leads/lib/page-locale";
import { translate } from "@/lib/i18n/messages";
import { localeAlternates } from "@/lib/seo/sitemap";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Public dealer application form (TEC-320): `/bayi-basvuru`. Open only
 * while the system setting `leads.dealer_application_enabled` is on
 * (TEC-317 config, default off); closed is a 404 and the page then is not
 * in the sitemap either. Language: `?lang=`, else cookie, else
 * Accept-Language.
 */
export const dynamic = "force-dynamic";

type PageProps = {
  searchParams: Promise<{ lang?: string | string[] }>;
};

/** One config read per request, shared by generateMetadata and the page. */
const config = cache(async () => {
  const requestHeaders = await headers();
  return fetchDealerApplicationConfig(
    {
      clientIp: clientIpFromHeaders(requestHeaders),
      forwardedHost: forwardedHostFromHeaders(requestHeaders),
    },
    fetchUpstream,
  );
});

export async function generateMetadata({
  searchParams,
}: PageProps): Promise<Metadata> {
  const { lang } = await searchParams;
  const { locale } = await publicPageLocale(lang);
  if ((await config()) !== "open") {
    return {
      title: translate(locale, "landing.dealer_application.not_found_title"),
      robots: { index: false, follow: false },
    };
  }
  const title = translate(locale, "landing.dealer_application.meta_title");
  const description = translate(
    locale,
    "landing.dealer_application.meta_description",
  );
  const path = routes.public.dealerApplication;
  return {
    title,
    description,
    alternates: {
      canonical: path,
      languages: localeAlternates(`${site.url}${path}`),
    },
    openGraph: {
      type: "website",
      siteName: site.name,
      title,
      description,
      locale,
      url: path,
      images: [{ ...site.ogImage, alt: title }],
    },
    twitter: { card: "summary", title, description },
  };
}

export default async function DealerApplicationPage({
  searchParams,
}: PageProps) {
  const { lang } = await searchParams;
  const { locale } = await publicPageLocale(lang);
  const state = await config();
  if (state === "closed") notFound();
  return <DealerApplicationView config={state} locale={locale} />;
}
