import { cookies, headers } from "next/headers";

import { i18nConfig, normalizeLocale, type AppLocale } from "@/config/i18n";
import { loadMessages } from "@/lib/i18n/messages";
import { resolveRequestLocale } from "@/lib/i18n/request-locale";

type LangParam = string | string[] | undefined;

/**
 * Language of a public page (TEC-320), same order as /garanti and the
 * landing: `?lang=`, else the language cookie, else Accept-Language. The
 * catalog is loaded before it returns.
 */
export async function publicPageLocale(
  lang?: LangParam,
): Promise<{ locale: AppLocale; timeZone: string }> {
  const store = await cookies();
  const requestHeaders = await headers();
  const resolved = resolveRequestLocale(
    (name) => store.get(name)?.value,
    requestHeaders.get("accept-language"),
  );
  const fromQuery = normalizeLocale(Array.isArray(lang) ? lang[0] : lang);
  const locale = fromQuery ?? resolved.locale;
  if (locale !== i18nConfig.fallbackLocale) await loadMessages(locale);
  return { locale, timeZone: resolved.timeZone };
}
