import {
  i18nConfig,
  localeDir,
  normalizeLocale,
  type AppLocale,
} from "@/config/i18n";
import { isValidTimeZone } from "@/lib/i18n/format";

export type RequestLocale = {
  locale: AppLocale;
  dir: "ltr" | "rtl";
  timeZone: string;
};

/**
 * First supported language of an Accept-Language header, by q-value
 * (ties keep header order; q=0 and "*" are skipped). Each tag is normalized
 * like the cookie ("de-AT" -> de, "zh_CN" -> zh-CN). Null when none matches.
 */
export function localeFromAcceptLanguage(
  header: string | null | undefined,
): AppLocale | null {
  if (!header) return null;
  const ranked = header
    .split(",")
    .map((part, index) => {
      const [tag = "", ...params] = part.trim().split(";");
      let q = 1;
      for (const param of params) {
        const [key, value] = param.trim().split("=");
        if (key?.trim().toLowerCase() === "q") {
          const parsed = Number.parseFloat(value ?? "");
          q = Number.isFinite(parsed) ? parsed : 0;
        }
      }
      return { tag: tag.trim(), q, index };
    })
    .filter((entry) => entry.tag && entry.tag !== "*" && entry.q > 0)
    .sort((a, b) => b.q - a.q || a.index - b.index);
  for (const entry of ranked) {
    const locale = normalizeLocale(entry.tag);
    if (locale) return locale;
  }
  return null;
}

/**
 * Language, direction and time zone of a server render. The language is
 * the NEXT_LOCALE cookie the LocaleProvider writes; without a (valid)
 * cookie, the browser's Accept-Language (anonymous first visit, TEC-142);
 * else the default (tr). The zone comes from NEXT_TIMEZONE. Unknown values
 * fall back to the defaults, so a tampered cookie cannot break rendering.
 */
export function resolveRequestLocale(
  readCookie: (name: string) => string | undefined,
  acceptLanguage?: string | null,
): RequestLocale {
  const locale =
    normalizeLocale(readCookie(i18nConfig.cookieName)) ??
    localeFromAcceptLanguage(acceptLanguage) ??
    i18nConfig.defaultLocale;
  const zone = readCookie(i18nConfig.timeZoneCookieName);
  const timeZone =
    zone && isValidTimeZone(zone) ? zone : i18nConfig.defaultTimeZone;
  return { locale, dir: localeDir(locale), timeZone };
}
