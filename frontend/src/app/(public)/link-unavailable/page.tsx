import type { Metadata } from "next";
import { cookies, headers } from "next/headers";

import { i18nConfig, localeDir, normalizeLocale } from "@/config/i18n";
import { ShortUrlUnavailable } from "@/features/short-urls/components/short-url-unavailable";
import { parseUnavailableReason } from "@/features/short-urls/lib/short-url";
import { loadMessages, translate } from "@/lib/i18n/messages";
import { resolveRequestLocale } from "@/lib/i18n/request-locale";

/**
 * Notice page of a short link that cannot be followed (TEC-249): expired
 * (Go 410), unknown (404) or a failed lookup. `/s/{token}` redirects here
 * with `?reason=`. No session. Language: `?lang=`, else the language
 * cookie, else Accept-Language.
 */
export const dynamic = "force-dynamic";

type PageProps = {
  searchParams: Promise<{
    lang?: string | string[];
    reason?: string | string[];
  }>;
};

async function pageLocale(searchParams: PageProps["searchParams"]) {
  const { lang } = await searchParams;
  const store = await cookies();
  const requestHeaders = await headers();
  const resolved = resolveRequestLocale(
    (name) => store.get(name)?.value,
    requestHeaders.get("accept-language"),
  );
  const locale =
    normalizeLocale(Array.isArray(lang) ? lang[0] : lang) ?? resolved.locale;
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  return locale;
}

export async function generateMetadata({
  searchParams,
}: PageProps): Promise<Metadata> {
  const locale = await pageLocale(searchParams);
  return {
    title: translate(locale, "common.short_link.page_title"),
    robots: { index: false, follow: false, nocache: true },
  };
}

export default async function LinkUnavailablePage({ searchParams }: PageProps) {
  const locale = await pageLocale(searchParams);
  const { reason } = await searchParams;
  return (
    <ShortUrlUnavailable
      reason={parseUnavailableReason(reason)}
      locale={locale}
      dir={localeDir(locale)}
    />
  );
}
