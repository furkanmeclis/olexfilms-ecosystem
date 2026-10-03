import type { Metadata } from "next";
import { cookies, headers } from "next/headers";

import { i18nConfig, normalizeLocale } from "@/config/i18n";
import { LandingView } from "@/features/landing/components/landing-view";
import { loadMessages, translate } from "@/lib/i18n/messages";
import { resolveRequestLocale } from "@/lib/i18n/request-locale";

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
  return {
    title: { absolute: translate(locale, "landing.meta.title") },
    description: translate(locale, "landing.meta.description"),
  };
}

export default async function PublicHomePage({ searchParams }: PageProps) {
  const { locale, fromQuery } = await pageLocale(searchParams);
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  return <LandingView locale={locale} lang={fromQuery ?? undefined} />;
}
