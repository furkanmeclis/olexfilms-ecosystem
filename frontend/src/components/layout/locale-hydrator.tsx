"use client";

import { useEffect } from "react";

import { normalizeLocale } from "@/config/i18n";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Applies Me.effective_locale / effective_timezone after bootstrap (user ->
 * organization -> brand center). The provider stores both in cookies so the
 * next server render starts in the same language and zone.
 */
export function LocaleHydrator() {
  const { user, bootstrapped } = useAuth();
  const { locale, setLocale, setTimeZone } = useLocale();
  const effectiveLocale = user?.locale;
  const effectiveTimeZone = user?.timeZone;

  useEffect(() => {
    if (!bootstrapped) return;
    const next = normalizeLocale(effectiveLocale);
    if (next && next !== locale) void setLocale(next);
    // Only the user value matters here; a manual switch updates the profile.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bootstrapped, effectiveLocale, setLocale]);

  useEffect(() => {
    if (bootstrapped) setTimeZone(effectiveTimeZone);
  }, [bootstrapped, effectiveTimeZone, setTimeZone]);

  return null;
}
