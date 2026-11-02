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

/** 0-9 to Arabic-Indic; numbers also get the Arabic group and decimal marks. */
export function toArabicDigits(value: string, numeric = false): string {
  const digits = value.replace(/[0-9]/g, (d) => String.fromCharCode(0x0660 + Number(d)));
  return numeric ? digits.replace(/(?<=[٠-٩]),(?=[٠-٩])/g, '٬').replace(/(?<=[٠-٩])\.(?=[٠-٩])/g, '٫') : digits;
}

type AnyFormatter = {
  format: (value?: never) => string;
  formatToParts?: (value?: never) => { type: string; value: string }[];
};

function wrapFormatter<T extends object>(ctor: T, numeric: boolean): T {
  const Base = ctor as unknown as new (locales?: unknown, options?: unknown) => AnyFormatter;
  const Wrapped = function (locales?: unknown, options?: unknown) {
    const formatter = new Base(locales, options);
    const tag = String(Array.isArray(locales) ? locales[0] : locales);
    const nu = /-u-nu-(arab|latn)/.exec(tag)?.[1];
    if (!nu) return formatter;
    const convert = (text: string) => (nu === 'arab' ? toArabicDigits(text, numeric) : toLatinDigits(text));
    return new Proxy(formatter, {
      get(target, key) {
        const member = Reflect.get(target, key) as unknown;
        if (typeof member !== 'function') return member;
        const bound = member.bind(target) as (...args: unknown[]) => unknown;
        if (key === 'format') return (...args: unknown[]) => convert(bound(...args) as string);
        if (key === 'formatToParts') {
          return (...args: unknown[]) =>
            (bound(...args) as { type: string; value: string }[]).map((p) => ({ ...p, value: convert(p.value) }));
        }
        return bound;
      },
    });
  };
  Object.assign(Wrapped, ctor);
  Wrapped.prototype = (ctor as unknown as { prototype: object }).prototype;
  (Wrapped as unknown as { supportedLocalesOf: unknown }).supportedLocalesOf = (
    ctor as unknown as { supportedLocalesOf?: unknown }
  ).supportedLocalesOf;
  return Wrapped as unknown as T;
}

/**
 * Hermes on iOS ignores the `-u-nu-` extension, so formatters built from
 * `intlLocale` would print Latin digits. Engines that honor it are left alone.
 */
export function ensureDigitSupport(intl: typeof Intl = Intl) {
  if (new intl.NumberFormat('ar-u-nu-arab').format(1) === '١') return;
  intl.NumberFormat = wrapFormatter(intl.NumberFormat, true);
  intl.DateTimeFormat = wrapFormatter(intl.DateTimeFormat, false);
}

ensureDigitSupport();

/**
 * A number in the language's digits (Arabic-Indic for Arabic unless Latin is
 * chosen). Four-digit values such as years get no grouping separator.
 */
export function formatDigits(value: number, locale: string): string {
  return new Intl.NumberFormat(intlLocale(locale), { useGrouping: Math.abs(value) >= 10000 }).format(value);
}
