"use client";

import { Direction } from "radix-ui";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import {
  i18nConfig,
  localeDir,
  normalizeLocale,
  type AppLocale,
} from "@/config/i18n";
import {
  createFormatter,
  isValidTimeZone,
  type Formatter,
} from "@/lib/i18n/format";
import {
  isLocaleLoaded,
  loadMessages,
  registerMessages,
  translate,
  translatePlural,
  type LocaleCatalog,
} from "@/lib/i18n/messages";

type LocaleContextValue = {
  locale: AppLocale;
  dir: "ltr" | "rtl";
  /** Effective IANA time zone used by `format`. */
  timeZone: string;
  /** Loads the language chunk, then switches (cookie + <html lang dir>). */
  setLocale: (locale: AppLocale) => Promise<void>;
  setTimeZone: (timeZone: string | null | undefined) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  tp: (
    key: string,
    count: number,
    params?: Record<string, string | number>,
  ) => string;
  /** Intl formatters bound to locale and time zone. */
  format: Formatter;
};

const LocaleContext = createContext<LocaleContextValue | null>(null);

const ONE_YEAR = 60 * 60 * 24 * 365;

function writeCookie(name: string, value: string) {
  const secure = window.location.protocol === "https:" ? "; Secure" : "";
  document.cookie = `${name}=${encodeURIComponent(value)}; Path=/; Max-Age=${ONE_YEAR}; SameSite=Lax${secure}`;
}

function hasCookie(name: string) {
  return document.cookie
    .split(";")
    .some((part) => part.trim().startsWith(`${name}=`));
}

function readStoredLocale(): AppLocale | null {
  try {
    return normalizeLocale(window.localStorage.getItem(i18nConfig.storageKey));
  } catch {
    return null;
  }
}

function applyDocumentLocale(locale: AppLocale) {
  document.documentElement.lang = locale;
  document.documentElement.dir = localeDir(locale);
}

type LocaleProviderProps = {
  children: ReactNode;
  /** Resolved on the server from the NEXT_LOCALE cookie. */
  initialLocale?: AppLocale;
  /** Catalog of initialLocale, loaded on the server (en is bundled). */
  initialMessages?: LocaleCatalog | null;
  /** Resolved on the server from the NEXT_TIMEZONE cookie. */
  initialTimeZone?: string;
};

export function LocaleProvider({
  children,
  initialLocale = i18nConfig.defaultLocale,
  initialMessages,
  initialTimeZone = i18nConfig.defaultTimeZone,
}: LocaleProviderProps) {
  // Same catalog on the server render and the hydration render, so the
  // first paint is already in the right language (no flash, no mismatch).
  if (initialMessages) registerMessages(initialLocale, initialMessages);

  const [locale, setLocaleState] = useState<AppLocale>(initialLocale);
  const [timeZone, setTimeZoneState] = useState(initialTimeZone);
  // Re-render once a catalog that SSR did not hand over has arrived.
  const [, setCatalogVersion] = useState(0);

  const setLocale = useCallback(async (next: AppLocale) => {
    try {
      await loadMessages(next);
    } catch {
      // Chunk failed to load: keys fall back to en.
    }
    writeCookie(i18nConfig.cookieName, next);
    try {
      window.localStorage.setItem(i18nConfig.storageKey, next);
    } catch {
      // Ignore storage errors in restricted/private modes
    }
    applyDocumentLocale(next);
    setLocaleState(next);
  }, []);

  const setTimeZone = useCallback((next: string | null | undefined) => {
    if (!next || !isValidTimeZone(next)) return;
    writeCookie(i18nConfig.timeZoneCookieName, next);
    setTimeZoneState(next);
  }, []);

  useEffect(() => {
    applyDocumentLocale(locale);
    if (isLocaleLoaded(locale)) return;
    let active = true;
    void loadMessages(locale)
      .then(() => active && setCatalogVersion((v) => v + 1))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [locale]);

  useEffect(() => {
    // Sessions from before the cookie existed kept the choice only in
    // localStorage: carry it over once.
    if (!hasCookie(i18nConfig.cookieName)) {
      const stored = readStoredLocale();
      if (stored && stored !== initialLocale) {
        queueMicrotask(() => void setLocale(stored));
      } else {
        writeCookie(i18nConfig.cookieName, initialLocale);
      }
    }
    // Another tab switched the language.
    const onStorage = (event: StorageEvent) => {
      if (event.key !== i18nConfig.storageKey) return;
      const next = normalizeLocale(event.newValue);
      if (next) void setLocale(next);
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, [initialLocale, setLocale]);

  const value = useMemo<LocaleContextValue>(
    () => ({
      locale,
      dir: localeDir(locale),
      timeZone,
      setLocale,
      setTimeZone,
      t: (key, params) =>
        translate(locale, key, params, i18nConfig.fallbackLocale),
      tp: (key, count, params) => translatePlural(locale, key, count, params),
      format: createFormatter({ locale, timeZone }),
    }),
    [locale, timeZone, setLocale, setTimeZone],
  );

  return (
    <LocaleContext.Provider value={value}>
      <Direction.DirectionProvider dir={value.dir}>
        {children}
      </Direction.DirectionProvider>
    </LocaleContext.Provider>
  );
}

export function useLocale() {
  const ctx = useContext(LocaleContext);
  if (!ctx) throw new Error("useLocale must be used within LocaleProvider");
  return ctx;
}

const defaultFormatter = createFormatter({
  locale: i18nConfig.defaultLocale,
  timeZone: i18nConfig.defaultTimeZone,
});

/** Formatters for shared UI that may render outside LocaleProvider. */
export function useFormatter(): Formatter {
  return useContext(LocaleContext)?.format ?? defaultFormatter;
}
