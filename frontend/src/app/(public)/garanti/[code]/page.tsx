import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { cache } from "react";

import { i18nConfig, localeDir, normalizeLocale } from "@/config/i18n";
import { PublicWarrantyView } from "@/features/warranty/components/public-warranty-view";
import {
  fetchPublicWarranty,
  parsePdfNotice,
} from "@/features/warranty/lib/public-warranty";
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
  searchParams: Promise<{
    lang?: string | string[];
    pdf?: string | string[];
  }>;
};

/**
 * One Go lookup per request: generateMetadata (OG tags) and the page share
 * it, so the per-IP limit is hit once per view.
 */
const lookup = cache(async (code: string) => {
  const requestHeaders = await headers();
  return fetchPublicWarranty(
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
  return { locale, dir: localeDir(locale), timeZone: resolved.timeZone };
}

/**
 * Title and Open Graph tags (TEC-248): a shared QR / WhatsApp link previews
 * the brand, product and status only, never the vehicle or the code.
 */
export async function generateMetadata({
  params,
  searchParams,
}: PageProps): Promise<Metadata> {
  const { code } = await params;
  const { locale } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  const pageTitle = translate(locale, "warranty.public.page_title");
  const result = await lookup(code);
  let title = pageTitle;
  let description = translate(locale, "warranty.public.not_found_body");
  if (result.kind === "ok") {
    const w = result.warranty;
    title = `${w.product.name} · ${w.brand.name}`;
    description = translate(locale, "warranty.public.og_description", {
      brand: w.brand.name,
      product: w.product.name,
      status: translate(locale, `warranty.public.status.${w.status}`),
    });
  } else if (result.kind === "not_found") {
    title = translate(locale, "warranty.public.not_found_title");
  } else {
    description = pageTitle;
  }
  return {
    title,
    description,
    openGraph: {
      type: "website",
      title,
      description,
      siteName: result.kind === "ok" ? result.warranty.brand.name : undefined,
      locale,
    },
    twitter: { card: "summary", title, description },
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
  const { pdf } = await searchParams;
  const result = await lookup(code);

  return (
    <PublicWarrantyView
      result={result}
      locale={locale}
      timeZone={timeZone}
      path={`/garanti/${encodeURIComponent(code)}`}
      pdfNotice={parsePdfNotice(pdf)}
    />
  );
}
