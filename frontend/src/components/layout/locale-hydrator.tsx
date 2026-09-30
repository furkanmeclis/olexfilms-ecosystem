"use client";

import { useEffect } from "react";

import type { AppLocale } from "@/config/i18n";
import { i18nConfig } from "@/config/i18n";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/** Applies persisted user locale from Me over localStorage after bootstrap. */
export function LocaleHydrator() {
  const { user, bootstrapped } = useAuth();
  const { setLocale } = useLocale();

  useEffect(() => {
    if (!bootstrapped || !user?.locale) return;
    const dbLocale = user.locale as AppLocale;
    if (i18nConfig.supportedLocales.includes(dbLocale)) {
      setLocale(dbLocale);
    }
  }, [bootstrapped, setLocale, user?.locale]);

  return null;
}
