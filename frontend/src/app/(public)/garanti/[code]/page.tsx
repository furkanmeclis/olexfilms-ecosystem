import type { Metadata } from "next";
import { cookies, headers } from "next/headers";

import { i18nConfig, localeDir, normalizeLocale } from "@/config/i18n";
import { PublicWarrantyView } from "@/features/warranty/components/public-warranty-view";
import { fetchPublicWarranty } from "@/features/warranty/lib/public-warranty";
import { loadMessages, translate } from "@/lib/i18n/messages";
import { resolveRequestLocale } from "@/lib/i18n/request-locale";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Public warranty page (TEC-189), the target of the warranty QR code and
 * the notification links (PUBLIC_FRONTEND_URL/garanti/{public_code}).
 * No session: rendered on the server from Go's public lookup, which rate
 * limits per client IP and returns no personal data. Language: `?lang=`,
 * else the language cookie, else the browser's Accept-Language.
 */
export const dynamic = "force-dynamic";

type PageProps = {
  params: Promise<{ code: string }>;
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
  const locale = fromQuery ?? resolved.locale;
  return { locale, dir: localeDir(locale), timeZone: resolved.timeZone };
}

export async function generateMetadata({
  searchParams,
}: PageProps): Promise<Metadata> {
  const { locale } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  return {
    title: translate(locale, "warranty.public.page_title"),
    robots: { index: false, follow: false, nocache: true },
    // The code in the URL is the access key: never send it to other sites.
    referrer: "no-referrer",
  };
}

export default async function PublicWarrantyPage({
  params,
  searchParams,
}: PageProps) {
  const { code } = await params;
  const { locale, timeZone } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);

  const requestHeaders = await headers();
  const result = await fetchPublicWarranty(
    code,
    {
      clientIp: clientIpFromHeaders(requestHeaders),
      forwardedHost: forwardedHostFromHeaders(requestHeaders),
    },
    fetchUpstream,
  );

  return (
    <PublicWarrantyView
      result={result}
      locale={locale}
      timeZone={timeZone}
      path={`/garanti/${encodeURIComponent(code)}`}
    />
  );
}
