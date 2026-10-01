import { i18nConfig, type AppLocale } from "@/config/i18n";
import { catalogLoaders } from "@/lib/i18n/catalog-loaders";
import type { LocaleCatalog, MessageDictionary } from "@/lib/i18n/types";
import en from "@/locales/en";

export type { LocaleCatalog, MessageDictionary };

/**
 * Loaded languages. en is bundled (every missing key falls back to it); the
 * other languages are fetched on demand with loadMessages() (one chunk per
 * language) or handed over by the server through registerMessages().
 */
const catalogs = new Map<AppLocale, LocaleCatalog>([["en", en]]);
const pending = new Map<AppLocale, Promise<LocaleCatalog>>();

export function isLocaleLoaded(locale: AppLocale): boolean {
  return catalogs.has(locale);
}

export function getLoadedMessages(locale: AppLocale): LocaleCatalog | null {
  return catalogs.get(locale) ?? null;
}

/** Stores a catalog that was loaded elsewhere (SSR props). Idempotent. */
export function registerMessages(locale: AppLocale, catalog: LocaleCatalog) {
  if (!catalogs.has(locale)) catalogs.set(locale, catalog);
}

/** Loads a language's catalog once; concurrent calls share the request. */
export function loadMessages(locale: AppLocale): Promise<LocaleCatalog> {
  const ready = catalogs.get(locale);
  if (ready) return Promise.resolve(ready);
  let request = pending.get(locale);
  if (!request) {
    request = catalogLoaders[locale]()
      .then((catalog) => {
        catalogs.set(locale, catalog);
        return catalog;
      })
      .finally(() => pending.delete(locale));
    pending.set(locale, request);
  }
  return request;
}

function lookup(locale: AppLocale, ns: string, path: string) {
  return catalogs.get(locale)?.[ns]?.[path];
}

function interpolate(text: string, params?: Record<string, string | number>) {
  if (!params) return text;
  let out = text;
  for (const [k, v] of Object.entries(params)) {
    out = out.replace(new RegExp(`{{\\s*${k}\\s*}}`, "g"), String(v));
  }
  return out;
}

export function translate(
  locale: AppLocale,
  key: string,
  params?: Record<string, string | number>,
  fallbackLocale: AppLocale = i18nConfig.fallbackLocale,
): string {
  const [ns, ...rest] = key.split(".");
  const path = rest.join(".");
  const primary = lookup(locale, ns, path);
  const fallback = lookup(fallbackLocale, ns, path);
  if (primary === undefined)
    recordMissingKey(locale, key, fallback !== undefined);
  return interpolate(primary ?? fallback ?? key, params);
}

const pluralRules = new Map<string, Intl.PluralRules>();

/** CLDR plural category of count in locale (ar: zero/one/two/few/many/other). */
export function pluralCategory(
  locale: AppLocale,
  count: number,
): Intl.LDMLPluralRule {
  let rules = pluralRules.get(locale);
  if (!rules) {
    rules = new Intl.PluralRules(locale);
    pluralRules.set(locale, rules);
  }
  return rules.select(count);
}

/**
 * Plural lookup with Intl.PluralRules: `key_<category>` in the active
 * language (`_zero/_one/_two/_few/_many/_other`), then the fallback
 * language's own category, then `key_other`. `{{count}}` is always set.
 */
export function translatePlural(
  locale: AppLocale,
  key: string,
  count: number,
  params?: Record<string, string | number>,
  fallbackLocale: AppLocale = i18nConfig.fallbackLocale,
): string {
  const [ns, ...rest] = key.split(".");
  const path = rest.join(".");
  const values = { count, ...params };
  const candidates: [AppLocale, string][] = [
    [locale, `${path}_${pluralCategory(locale, count)}`],
    [locale, `${path}_other`],
    [fallbackLocale, `${path}_${pluralCategory(fallbackLocale, count)}`],
    [fallbackLocale, `${path}_other`],
  ];
  for (const [loc, p] of candidates) {
    const text = lookup(loc, ns, p);
    if (text !== undefined) return interpolate(text, values);
  }
  recordMissingKey(locale, key, false);
  return key;
}

/**
 * Development aid: remembers keys the active locale could not resolve so a
 * crawler (or `window.__i18nMissing` in devtools) can list them. No-op in
 * production builds.
 */
function recordMissingKey(
  locale: AppLocale,
  key: string,
  hasFallback: boolean,
) {
  if (process.env.NODE_ENV === "production" || typeof window === "undefined")
    return;
  const w = window as unknown as { __i18nMissing?: Record<string, string> };
  w.__i18nMissing ??= {};
  w.__i18nMissing[`${locale}:${key}`] = hasFallback ? "fallback" : "missing";
}
