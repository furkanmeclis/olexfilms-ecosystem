"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";

import { i18nConfig, type AppLocale } from "@/config/i18n";
import { translate, translatePlural } from "@/lib/i18n/messages";

type LocaleContextValue = {
  locale: AppLocale;
  dir: "ltr" | "rtl";
  setLocale: (locale: AppLocale) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  tp: (
    key: string,
    count: number,
    params?: Record<string, string | number>,
  ) => string;
};

const LocaleContext = createContext<LocaleContextValue | null>(null);

const LOCALE_CHANGE_EVENT = "app:locale-change";

function resolveDir(locale: AppLocale): "ltr" | "rtl" {
  return i18nConfig.rtlLocales.includes(locale) ? "rtl" : "ltr";
}

function readStoredLocale(): AppLocale | null {
  if (typeof window === "undefined") return null;
  try {
    const stored = window.localStorage.getItem(
      i18nConfig.storageKey,
    ) as AppLocale | null;
    if (stored && i18nConfig.supportedLocales.includes(stored)) {
      return stored;
    }
  } catch {
    return null;
  }
  return null;
}

function getClientLocale(): AppLocale {
  return readStoredLocale() ?? i18nConfig.defaultLocale;
}

function getServerLocale(): AppLocale {
  return i18nConfig.defaultLocale;
}

function subscribeLocale(onStoreChange: () => void) {
  const handleStorage = (event: StorageEvent) => {
    if (event.key === i18nConfig.storageKey || event.key === null) {
      onStoreChange();
    }
  };
  const handleCustom = () => onStoreChange();
  window.addEventListener("storage", handleStorage);
  window.addEventListener(LOCALE_CHANGE_EVENT, handleCustom);
  return () => {
    window.removeEventListener("storage", handleStorage);
    window.removeEventListener(LOCALE_CHANGE_EVENT, handleCustom);
  };
}

export function LocaleProvider({ children }: { children: ReactNode }) {
  // SSR and the first client render share defaultLocale via getServerSnapshot.
  // Client preference comes from localStorage without a mount-time setState.
  const locale = useSyncExternalStore(
    subscribeLocale,
    getClientLocale,
    getServerLocale,
  );

  const setLocale = useCallback((next: AppLocale) => {
    try {
      window.localStorage.setItem(i18nConfig.storageKey, next);
    } catch {
      // Ignore storage errors in restricted/private modes
    }
    document.documentElement.lang = next;
    document.documentElement.dir = resolveDir(next);
    window.dispatchEvent(new Event(LOCALE_CHANGE_EVENT));
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dir = resolveDir(locale);
  }, [locale]);

  const value = useMemo<LocaleContextValue>(
    () => ({
      locale,
      dir: resolveDir(locale),
      setLocale,
      t: (key, params) =>
        translate(locale, key, params, i18nConfig.fallbackLocale),
      tp: (key, count, params) => translatePlural(locale, key, count, params),
    }),
    [locale, setLocale],
  );

  return (
    <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
  );
}

export function useLocale() {
  const ctx = useContext(LocaleContext);
  if (!ctx) throw new Error("useLocale must be used within LocaleProvider");
  return ctx;
}
