/**
 * Digits of formatted numbers (TEC-1009). Arabic shows Arabic-Indic digits
 * (٠١٢٣٤٥٦٧٨٩) by default; Settings can switch to Latin digits (the Maghreb
 * convention). Every Intl formatter takes its locale through `intlLocale`.
 * Identifiers (plate, VIN, barcode, service number, phone, codes, URLs) are
 * printed as is and keep Latin digits; typed Arabic-Indic or Persian digits
 * are stored as Latin (`toLatinDigits`, applied by the Input component).
 *
 * Kept free of React / storage imports so plain node tests can use it; the
 * preferences store sets the style.
 */
export type DigitStyle = 'arab' | 'latn';

let current: DigitStyle = 'arab';

export function setDigitStyle(style: DigitStyle) {
  current = style;
}

export function digitStyle(): DigitStyle {
  return current;
}

export function hasDigitChoice(locale: string): boolean {
  return locale.split('-')[0]?.toLowerCase() === 'ar';
}

/** The locale tag Intl gets: Arabic with an explicit numbering system. */
export function intlLocale(locale: string, digits: DigitStyle = current): string {
  return hasDigitChoice(locale) ? `${locale}-u-nu-${digits}` : locale;
}

const ARABIC_INDIC = /[٠-٩۰-۹]/g;

/** Arabic-Indic (٠-٩) and Persian (۰-۹) digits to 0-9. */
export function toLatinDigits(value: string): string {
  return value.replace(ARABIC_INDIC, (d) => {
    const code = d.charCodeAt(0);
    return String(code >= 0x06f0 ? code - 0x06f0 : code - 0x0660);
  });
}
