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
 * Language, direction and time zone of a server render, from the cookies
 * the LocaleProvider writes (NEXT_LOCALE, NEXT_TIMEZONE). Unknown values
 * fall back to the defaults, so a tampered cookie cannot break rendering.
 */
export function resolveRequestLocale(
  readCookie: (name: string) => string | undefined,
): RequestLocale {
  const locale =
    normalizeLocale(readCookie(i18nConfig.cookieName)) ??
    i18nConfig.defaultLocale;
  const zone = readCookie(i18nConfig.timeZoneCookieName);
  const timeZone =
    zone && isValidTimeZone(zone) ? zone : i18nConfig.defaultTimeZone;
  return { locale, dir: localeDir(locale), timeZone };
}
