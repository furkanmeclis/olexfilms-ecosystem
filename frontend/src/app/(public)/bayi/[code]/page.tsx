import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { cache } from "react";

import { i18nConfig, localeDir, normalizeLocale } from "@/config/i18n";
import { routes } from "@/config/routes";
import { site } from "@/config/site";
import { DealerShowcaseView } from "@/features/dealers/components/dealer-showcase-view";
import {
  dealerLocality,
  dealerLogoSrc,
  fetchPublicDealer,
} from "@/features/dealers/lib/dealer-showcase";
import { loadMessages, translate } from "@/lib/i18n/messages";
import { resolveRequestLocale } from "@/lib/i18n/request-locale";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Public dealer showcase (TEC-250): `/bayi/{code}`, code = organization
 * slug. No session: rendered on the server from Go's public lookup
 * (`GET /v1/public/dealers/{code}`), which rate limits per client IP and
 * returns showcase fields only. Language: `?lang=`, else the language
 * cookie, else the browser's Accept-Language.
 */
export const dynamic = "force-dynamic";

type PageProps = {
  params: Promise<{ code: string }>;
  searchParams: Promise<{ lang?: string | string[] }>;
};

/** One Go lookup per request, shared by generateMetadata and the page. */
const lookup = cache(async (code: string) => {
  const requestHeaders = await headers();
  return fetchPublicDealer(
    code,
    {
      clientIp: clientIpFromHeaders(requestHeaders),
      forwardedHost: forwardedHostFromHeaders(requestHeaders),
    },
    fetchUpstream,
  );
});

async function pageLocale(searchParams: PageProps["searchParams"]) {
  const { lang } = await searchParams;
  const store = await cookies();
  const requestHeaders = await headers();
  const resolved = resolveRequestLocale(
    (name) => store.get(name)?.value,
    requestHeaders.get("accept-language"),
  );
  const fromQuery = normalizeLocale(Array.isArray(lang) ? lang[0] : lang);
  const locale = fromQuery ?? resolved.locale;
  return { locale, dir: localeDir(locale) };
}

/** Title and Open Graph tags: dealer name, locality and logo. */
export async function generateMetadata({
  params,
  searchParams,
}: PageProps): Promise<Metadata> {
  const { code } = await params;
  const { locale } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  const result = await lookup(code);
  if (result.kind !== "ok") {
    const title =
      result.kind === "not_found"
        ? translate(locale, "portal.dealer_page.not_found_title")
        : translate(locale, "portal.dealer_page.error_title");
    return { title, robots: { index: false, follow: false } };
  }
  const dealer = result.dealer;
  const locality = dealerLocality(dealer);
  const description = locality
    ? translate(locale, "portal.dealer_page.og_description", {
        name: dealer.name,
        locality,
      })
    : translate(locale, "portal.dealer_page.og_description_short", {
        name: dealer.name,
      });
  const logo = dealerLogoSrc(dealer);
  return {
    title: dealer.name,
    description,
    alternates: { canonical: routes.public.dealer(dealer.code) },
    openGraph: {
      type: "website",
      title: dealer.name,
      description,
      locale,
      url: routes.public.dealer(dealer.code),
      images: logo
        ? [{ url: logo, alt: dealer.name }]
        : [{ ...site.ogImage, alt: dealer.name }],
    },
    twitter: { card: "summary", title: dealer.name, description },
  };
}

export default async function DealerShowcasePage({
  params,
  searchParams,
}: PageProps) {
  const { code } = await params;
  const { locale } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  const result = await lookup(code);
  const path =
    result.kind === "ok"
      ? routes.public.dealer(result.dealer.code)
      : routes.public.dealer(code);
  return <DealerShowcaseView result={result} locale={locale} path={path} />;
}
