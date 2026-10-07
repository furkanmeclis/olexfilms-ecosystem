import type { Metadata } from "next";
import { headers } from "next/headers";
import { notFound } from "next/navigation";
import { cache } from "react";

import { PublicQuoteView } from "@/features/public-leads/components/public-quote-view";
import { publicPageLocale } from "@/features/public-leads/lib/page-locale";
import {
  fetchPublicQuote,
  parseQuotePdfNotice,
  publicQuotePath,
} from "@/features/public-leads/lib/public-quote";
import { translate } from "@/lib/i18n/messages";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Public quote page (TEC-320), the target of the quote link Go sends
 * (TEC-315/316): `/teklif/{token}`. No session: rendered on the server
 * from Go's public lookup, which rate limits per client IP and never
 * returns the recipient's phone or e-mail. An unknown token is a real 404
 * (not-found.tsx). Language: `?lang=`, else cookie, else Accept-Language.
 */
export const dynamic = "force-dynamic";

type PageProps = {
  params: Promise<{ token: string }>;
  searchParams: Promise<{
    lang?: string | string[];
    pdf?: string | string[];
  }>;
};

/** One Go lookup per request, shared by generateMetadata and the page. */
const lookup = cache(async (token: string) => {
  const requestHeaders = await headers();
  return fetchPublicQuote(
    token,
    {
      clientIp: clientIpFromHeaders(requestHeaders),
      forwardedHost: forwardedHostFromHeaders(requestHeaders),
    },
    fetchUpstream,
  );
});

/** Title only; the token is the access key, so never indexed or referred. */
export async function generateMetadata({
  params,
  searchParams,
}: PageProps): Promise<Metadata> {
  const { token } = await params;
  const { lang } = await searchParams;
  const { locale } = await publicPageLocale(lang);
  const result = await lookup(token);
  const title =
    result.kind === "ok"
      ? `${translate(locale, "landing.quote.page_title")} ${result.quote.display_no} · ${result.quote.organization_name}`
      : result.kind === "not_found"
        ? translate(locale, "landing.quote.not_found_title")
        : translate(locale, "landing.quote.page_title");
  return {
    title,
    robots: { index: false, follow: false, nocache: true },
    referrer: "no-referrer",
  };
}

export default async function PublicQuotePage({
  params,
  searchParams,
}: PageProps) {
  const { token } = await params;
  const { lang, pdf } = await searchParams;
  const { locale, timeZone } = await publicPageLocale(lang);
  const result = await lookup(token);
  if (result.kind === "not_found") notFound();
  return (
    <PublicQuoteView
      result={result}
      locale={locale}
      timeZone={timeZone}
      token={token}
      path={publicQuotePath(token)}
      pdfNotice={parseQuotePdfNotice(pdf)}
    />
  );
}
