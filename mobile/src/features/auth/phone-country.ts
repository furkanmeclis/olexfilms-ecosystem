import countries from 'i18n-iso-countries/index.js';
import ar from 'i18n-iso-countries/langs/ar.json' with { type: 'json' };
import az from 'i18n-iso-countries/langs/az.json' with { type: 'json' };
import bg from 'i18n-iso-countries/langs/bg.json' with { type: 'json' };
import de from 'i18n-iso-countries/langs/de.json' with { type: 'json' };
import el from 'i18n-iso-countries/langs/el.json' with { type: 'json' };
import en from 'i18n-iso-countries/langs/en.json' with { type: 'json' };
import es from 'i18n-iso-countries/langs/es.json' with { type: 'json' };
import fr from 'i18n-iso-countries/langs/fr.json' with { type: 'json' };
import it from 'i18n-iso-countries/langs/it.json' with { type: 'json' };
import ru from 'i18n-iso-countries/langs/ru.json' with { type: 'json' };
import tr from 'i18n-iso-countries/langs/tr.json' with { type: 'json' };
import uk from 'i18n-iso-countries/langs/uk.json' with { type: 'json' };
import zh from 'i18n-iso-countries/langs/zh.json' with { type: 'json' };
import {
  getCountries,
  getCountryCallingCode,
  getExampleNumber,
  parsePhoneNumberFromString,
  type CountryCode,
} from 'libphonenumber-js/min';
import examples from 'libphonenumber-js/mobile/examples';

// Hermes has no Intl.DisplayNames, so region names come from a pure-JS package
// with only the app's locales registered.
for (const locale of [ar, az, bg, de, el, en, es, fr, it, ru, tr, uk, zh]) {
  countries.registerLocale(locale);
}

const SUPPORTED = new Set(['ar', 'az', 'bg', 'de', 'el', 'en', 'es', 'fr', 'it', 'ru', 'tr', 'uk', 'zh']);

function languageOf(locale: string): string {
  const lang = locale.split(/[-_]/)[0].toLowerCase();
  return SUPPORTED.has(lang) ? lang : 'en';
}

export type Country = {
  code: CountryCode;
  name: string;
  dial: string;
  flag: string;
};

export const DEFAULT_COUNTRY: CountryCode = 'TR';

/**
 * Normalizes user input to E.164 for the selected country. A number typed
 * with a leading `+` keeps its own country, so a pasted international number
 * wins over the picker. Returns null when the number is not valid.
 */
export function parsePhone(raw: string, country: CountryCode): string | null {
  const parsed = parsePhoneNumberFromString(raw, country);
  return parsed?.isValid() ? parsed.number : null;
}

/**
 * Country of a number typed in international form (`+44 7400 …`), so the
 * picker can follow a pasted number. Undefined until the number is complete.
 */
export function detectCountry(raw: string): CountryCode | undefined {
  if (!raw.trim().startsWith('+')) return undefined;
  const parsed = parsePhoneNumberFromString(raw);
  return parsed?.isValid() ? parsed.country : undefined;
}

/** A sample mobile number in national format, for the field placeholder. */
export function examplePhone(country: CountryCode): string | undefined {
  return getExampleNumber(country, examples)?.formatNational();
}

function flagOf(code: string): string {
  return String.fromCodePoint(
    ...[...code.toUpperCase()].map((char) => 0x1f1e6 + char.charCodeAt(0) - 65),
  );
}

export function listCountries(locale: string): Country[] {
  const lang = languageOf(locale);
  const first: Country[] = [];
  const rest: Country[] = [];
  for (const code of getCountries()) {
    // A few dialing regions (AC Ascension, TA Tristan da Cunha…) have no name in
    // some locales: fall back to English, and leave out a region with no name at all.
    const name = countries.getName(code, lang) ?? countries.getName(code, 'en');
    if (!name) continue;
    const country: Country = {
      code,
      name,
      dial: `+${getCountryCallingCode(code)}`,
      flag: flagOf(code),
    };
    (code === DEFAULT_COUNTRY ? first : rest).push(country);
  }
  rest.sort((a, b) => a.name.localeCompare(b.name, lang));
  return [...first, ...rest];
}

/** Combining diacritical marks (U+0300–U+036F); a plain range, since Hermes may lack `\p{…}` escapes. */
const MARKS = /[\u0300-\u036f]/g;

/** Case- and accent-insensitive key, so "turkiye" finds "Türkiye" and "İ" finds "i". */
function fold(v: string): string {
  return v.normalize('NFD').replace(MARKS, '').toLowerCase();
}

/** Search by name, ISO code (`gb`) or calling code (`44`, `+44`). */
export function filterCountries(list: Country[], query: string): Country[] {
  const q = fold(query.trim());
  if (!q) return list;
  const digits = q.replace(/\D/g, '');
  const dialOnly = /^\+?\d+$/.test(q);
  return list.filter((c) => {
    if (dialOnly) return c.dial.slice(1).startsWith(digits);
    return fold(c.name).includes(q) || c.code.toLowerCase() === q;
  });
}
