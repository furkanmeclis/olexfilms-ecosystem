/**
 * Supported UI languages (K10: 12 languages + Arabic). Canonical codes are
 * short BCP-47 tags that Intl accepts as is; the backend uses the same list
 * (backend/internal/platform/i18n, OpenAPI `Locale`).
 *
 * scripts/check-i18n.mjs reads this array, keep it a plain literal list.
 */
export const SUPPORTED_LOCALES = [
  "tr",
  "en",
  "bg",
  "de",
  "el",
  "uk",
  "ru",
  "fr",
  "es",
  "it",
  "zh-CN",
  "az",
  "ar",
] as const;

export type AppLocale = (typeof SUPPORTED_LOCALES)[number];

/** Each language in its own name (endonym), for the language switcher. */
export const LOCALE_NAMES: Record<AppLocale, string> = {
  tr: "Türkçe",
  en: "English",
  bg: "Български",
  de: "Deutsch",
  el: "Ελληνικά",
  uk: "Українська",
  ru: "Русский",
  fr: "Français",
  es: "Español",
  it: "Italiano",
  "zh-CN": "简体中文",
  az: "Azərbaycan dili",
  ar: "العربية",
};

const RTL_LOCALES: readonly AppLocale[] = ["ar"];

/**
 * Maps any spelling to a supported locale, like the backend's
 * i18n.Normalize: trim, "_" -> "-", case-insensitive exact match
 * ("zh_cn" -> zh-CN), else the primary language ("tr-TR" -> tr,
 * "zh" -> zh-CN). Unknown or empty input -> null.
 */
export function normalizeLocale(
  raw: string | null | undefined,
): AppLocale | null {
  const value = (raw ?? "").trim().replaceAll("_", "-").toLowerCase();
  if (!value) return null;
  const exact = SUPPORTED_LOCALES.find((l) => l.toLowerCase() === value);
  if (exact) return exact;
  const primary = value.split("-")[0];
  if (primary === "zh") return "zh-CN";
  return SUPPORTED_LOCALES.find((l) => l === primary) ?? null;
}

export function isRtlLocale(locale: AppLocale): boolean {
  return RTL_LOCALES.includes(locale);
}

export function localeDir(locale: AppLocale): "ltr" | "rtl" {
  return isRtlLocale(locale) ? "rtl" : "ltr";
}

export const i18nConfig = {
  defaultLocale:
    normalizeLocale(process.env.NEXT_PUBLIC_DEFAULT_LOCALE) ?? "tr",
  /** Always bundled; a key missing in the active language falls back here. */
  fallbackLocale: "en",
  supportedLocales: SUPPORTED_LOCALES,
  storageKey: "app.locale",
  /** Read on the server so <html lang dir> is right on the first paint. */
  cookieName: "NEXT_LOCALE",
  /** Effective IANA time zone (Me.effective_timezone), for SSR formatting. */
  timeZoneCookieName: "NEXT_TIMEZONE",
  defaultTimeZone: "Europe/Istanbul",
  rtlLocales: RTL_LOCALES,
} as const satisfies {
  defaultLocale: AppLocale;
  fallbackLocale: AppLocale;
  [key: string]: unknown;
};
