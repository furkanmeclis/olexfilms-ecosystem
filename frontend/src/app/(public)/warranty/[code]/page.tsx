import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { permanentRedirect } from "next/navigation";
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
 * Public warranty page (TEC-189), one page per service (TEC-1005): the
 * target of the printed warranty cards' QR (olexfilms.app/warranty/{no}),
 * the certificate QR and the notification links. A warranty public code
 * (per-warranty links before TEC-1005) resolves to its service and the
 * page moves (308) to /warranty/{service_no}.
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
    const s = result.service;
    const products = s.warranties.map((w) => w.product.name).join(", ");
    const active = s.warranties.some((w) => w.status === "active");
    title = `${products} · ${s.brand.name}`;
    description = translate(locale, "warranty.public.og_description", {
      brand: s.brand.name,
      product: products,
      status: translate(
        locale,
        `warranty.public.status.${active ? "active" : (s.warranties[0]?.status ?? "expired")}`,
      ),
    });
  } else if (result.kind === "not_found") {
    title = translate(locale, "warranty.public.not_found_title");
  } else {
    description = pageTitle;
  }
  return {
    // The document title stays the page name; the preview title carries
    // the product.
    title: pageTitle,
    description,
    openGraph: {
      type: "website",
      title,
      description,
      siteName: result.kind === "ok" ? result.service.brand.name : undefined,
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
  const { pdf, lang } = await searchParams;
  const result = await lookup(code);
  if (result.kind === "ok" && result.service.service_no !== code) {
    const q = Array.isArray(lang) ? lang[0] : lang;
    permanentRedirect(
      `/warranty/${encodeURIComponent(result.service.service_no)}${q ? `?lang=${encodeURIComponent(q)}` : ""}`,
    );
  }

  return (
    <PublicWarrantyView
      result={result}
      locale={locale}
      timeZone={timeZone}
      path={`/warranty/${encodeURIComponent(code)}`}
      pdfNotice={parsePdfNotice(pdf)}
    />
  );
}
