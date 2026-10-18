import {
  getCountries,
  getCountryCallingCode,
  parsePhoneNumberFromString,
  type CountryCode,
} from 'libphonenumber-js/min';

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

function flagOf(code: string): string {
  return String.fromCodePoint(
    ...[...code.toUpperCase()].map((char) => 0x1f1e6 + char.charCodeAt(0) - 65),
  );
}

export function listCountries(locale: string): Country[] {
  const names = new Intl.DisplayNames([locale], { type: 'region' });
  const all = getCountries().map((code) => ({
    code,
    name: names.of(code) ?? code,
    dial: `+${getCountryCallingCode(code)}`,
    flag: flagOf(code),
  }));
  const first = all.filter((c) => c.code === DEFAULT_COUNTRY);
  const rest = all
    .filter((c) => c.code !== DEFAULT_COUNTRY)
    .sort((a, b) => a.name.localeCompare(b.name, locale));
  return [...first, ...rest];
}
