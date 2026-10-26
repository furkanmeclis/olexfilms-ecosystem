/**
 * Digits of formatted numbers (TEC-1009). Arabic shows Arabic-Indic digits
 * (٠١٢٣٤٥٦٧٨٩) by default; a user may pick Latin digits (the Maghreb
 * convention) in the settings, kept in the NEXT_DIGITS cookie. Every Intl
 * formatter takes its locale through `intlLocale`, so dates, times,
 * numbers, money, percentages, measurements and counters follow the choice.
 *
 * Identifiers stay in Latin digits whatever the locale: they are printed
 * as is and never pass through Intl (see LATIN_ONLY). Typed Arabic-Indic or
 * Persian digits are stored as Latin (`toLatinDigits`, applied by Input).
 */

export type DigitStyle = "arab" | "latn";

export const DIGIT_STYLES: readonly DigitStyle[] = ["arab", "latn"];

export const DIGITS_COOKIE = "NEXT_DIGITS";

/** Values that keep Latin digits in every locale (shown verbatim). */
export const LATIN_ONLY = [
  "plate",
  "vin",
  "barcode",
  "service_no",
  "phone",
  "code",
  "id",
  "url",
] as const;

export function normalizeDigitStyle(
  raw: string | null | undefined,
): DigitStyle | null {
  return raw === "arab" || raw === "latn" ? raw : null;
}

/** Whether the locale has a digit choice (Arabic only). */
export function hasDigitChoice(locale: string): boolean {
  return locale.split("-")[0]?.toLowerCase() === "ar";
}

/**
 * The BCP-47 tag Intl gets: Arabic with Arabic-Indic digits unless the
 * user chose Latin (modern ICU defaults "ar" to Latin digits, so the
 * numbering system is explicit).
 */
export function intlLocale(
  locale: string,
  digits: DigitStyle = "arab",
): string {
  if (!hasDigitChoice(locale)) return locale;
  return `${locale}-u-nu-${digits}`;
}

const ARABIC_INDIC = /[٠-٩۰-۹]/g;

/** Arabic-Indic (٠-٩) and Persian (۰-۹) digits to 0-9. */
export function toLatinDigits(value: string): string {
  return value.replace(ARABIC_INDIC, (d) => {
    const code = d.charCodeAt(0);
    return String(code >= 0x06f0 ? code - 0x06f0 : code - 0x0660);
  });
}

/**
 * Latin digits of a hand-built text (e.g. "3 / 5") in the locale's digits.
 * Only for display text that is not an identifier (LATIN_ONLY).
 */
export function localizeDigits(
  value: string,
  locale: string,
  digits: DigitStyle = "arab",
): string {
  if (!hasDigitChoice(locale) || digits === "latn") return value;
  return value.replace(/[0-9]/g, (d) =>
    String.fromCharCode(0x0660 + Number(d)),
  );
}
