import { getLocales } from 'expo-localization';
import { createInstance } from 'i18next';
import { initReactI18next } from 'react-i18next';
import { I18nManager, Platform, Settings } from 'react-native';

import { storage } from '@/core/storage';

import { NAMESPACES, resources } from './resources';

/** K10: 12 languages + Arabic, same codes as the web and the backend `Locale`. */
export const LANGUAGES = [
  { code: 'tr', name: 'Türkçe' },
  { code: 'en', name: 'English' },
  { code: 'bg', name: 'Български' },
  { code: 'de', name: 'Deutsch' },
  { code: 'el', name: 'Ελληνικά' },
  { code: 'uk', name: 'Українська' },
  { code: 'ru', name: 'Русский' },
  { code: 'fr', name: 'Français' },
  { code: 'es', name: 'Español' },
  { code: 'it', name: 'Italiano' },
  { code: 'zh-CN', name: '简体中文' },
  { code: 'az', name: 'Azərbaycan dili' },
  { code: 'ar', name: 'العربية', rtl: true },
] as const;

export type Language = (typeof LANGUAGES)[number]['code'];

const PREF_KEY = 'pref.language';
const codes = LANGUAGES.map((l) => l.code) as readonly string[];

export function isLanguage(v: unknown): v is Language {
  return typeof v === 'string' && codes.includes(v);
}

export function isRTL(lang: string) {
  return LANGUAGES.some((l) => l.code === lang && 'rtl' in l && l.rtl);
}

/** Saved choice, else the first device language we support, else English. */
function detect(): Language {
  const saved = storage.getString(PREF_KEY);
  if (isLanguage(saved)) return saved;
  for (const l of getLocales()) {
    if (l.languageCode === 'zh') return 'zh-CN';
    if (isLanguage(l.languageCode)) return l.languageCode;
  }
  return 'en';
}

export const i18n = createInstance();

i18n.use(initReactI18next).init({
  resources,
  lng: detect(),
  fallbackLng: 'en',
  ns: NAMESPACES,
  defaultNS: 'common',
  // Keys are flat ("fields.email"), like the web catalogs.
  keySeparator: false,
  nsSeparator: ':',
  interpolation: { escapeValue: false },
  returnNull: false,
  initAsync: false,
});

/** iOS: keep the app language in step with a saved choice (applies from the next launch). */
function appleLanguage(lang: Language) {
  return lang === 'zh-CN' ? 'zh-Hans' : lang;
}
if (Platform.OS === 'ios') {
  const saved = storage.getString(PREF_KEY);
  const current = (Settings.get('AppleLanguages') as string[] | undefined)?.[0];
  if (isLanguage(saved) && current !== appleLanguage(saved)) {
    Settings.set({ AppleLanguages: [appleLanguage(saved)] });
  }
}

/** Whether the user ever picked a language (then the backend's locale is ignored). */
export function hasLanguagePreference() {
  return isLanguage(storage.getString(PREF_KEY));
}

/**
 * Changes the UI language. Returns true when the text direction changed: React
 * Native applies a new direction only after the app restarts.
 */
export async function setLanguage(lang: Language, { persist = true } = {}): Promise<boolean> {
  if (persist) storage.set(PREF_KEY, lang);
  await i18n.changeLanguage(lang);
  const rtl = isRTL(lang);
  if (Platform.OS === 'ios') {
    // expo-localization calls forceRTL(false) on every launch, so forceRTL never
    // sticks on iOS. The app's own language (AppleLanguages, like iOS per-app
    // language) does: with allowRTL on, React Native and UIKit (headers, tab
    // bar) lay out right-to-left from the next launch.
    if (persist) Settings.set({ AppleLanguages: [appleLanguage(lang)] });
  } else {
    I18nManager.allowRTL(rtl);
    I18nManager.forceRTL(rtl);
  }
  return rtl !== I18nManager.isRTL;
}

/** Translates a backend error code with the web's errors catalog. */
export function errorMessage(code: string | undefined, fallback?: string) {
  const key = `errors:codes.${code ?? ''}`;
  return code && i18n.exists(key) ? i18n.t(key) : (fallback ?? i18n.t('common:error_generic'));
}
