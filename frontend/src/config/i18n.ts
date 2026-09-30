export const i18nConfig = {
  defaultLocale: (process.env.NEXT_PUBLIC_DEFAULT_LOCALE ?? "tr") as
    "tr" | "en",
  fallbackLocale: (process.env.NEXT_PUBLIC_FALLBACK_LOCALE ?? "en") as
    "tr" | "en",
  supportedLocales: ["tr", "en"] as const,
  storageKey: "app.locale",
  /** RTL locales reserved for future languages */
  rtlLocales: [] as readonly string[],
} as const;

export type AppLocale = (typeof i18nConfig.supportedLocales)[number];
