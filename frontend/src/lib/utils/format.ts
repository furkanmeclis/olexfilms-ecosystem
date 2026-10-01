import { formatISO, type Locale as DateFnsLocale } from "date-fns";
import {
  ar as dfAr,
  az as dfAz,
  bg as dfBg,
  de as dfDe,
  el as dfEl,
  enUS as dfEnUS,
  es as dfEs,
  fr as dfFr,
  it as dfIt,
  ru as dfRu,
  tr as dfTr,
  uk as dfUk,
  zhCN as dfZhCN,
} from "date-fns/locale";
import {
  ar as dpAr,
  az as dpAz,
  bg as dpBg,
  de as dpDe,
  el as dpEl,
  enUS as dpEnUS,
  es as dpEs,
  fr as dpFr,
  it as dpIt,
  ru as dpRu,
  tr as dpTr,
  uk as dpUk,
  zhCN as dpZhCN,
} from "react-day-picker/locale";

import { i18nConfig, type AppLocale } from "@/config/i18n";

/*
 * date-fns locales are used by the calendar / date picker only. Dates and
 * numbers shown to the user go through Intl: `useLocale().format` and
 * `@/lib/i18n/format` (TEC-137).
 */
const DATE_FNS_LOCALES: Record<AppLocale, DateFnsLocale> = {
  tr: dfTr,
  en: dfEnUS,
  bg: dfBg,
  de: dfDe,
  el: dfEl,
  uk: dfUk,
  ru: dfRu,
  fr: dfFr,
  es: dfEs,
  it: dfIt,
  "zh-CN": dfZhCN,
  az: dfAz,
  ar: dfAr,
};

const DAY_PICKER_LOCALES = {
  tr: dpTr,
  en: dpEnUS,
  bg: dpBg,
  de: dpDe,
  el: dpEl,
  uk: dpUk,
  ru: dpRu,
  fr: dpFr,
  es: dpEs,
  it: dpIt,
  "zh-CN": dpZhCN,
  az: dpAz,
  ar: dpAr,
} satisfies Record<AppLocale, unknown>;

export function dateFnsLocale(
  locale: AppLocale = i18nConfig.defaultLocale,
): DateFnsLocale {
  return DATE_FNS_LOCALES[locale] ?? dfEnUS;
}

/** DayPicker locale (`react-day-picker/locale`) for calendar UI. */
export function dayPickerLocale(locale: AppLocale = i18nConfig.defaultLocale) {
  return DAY_PICKER_LOCALES[locale] ?? dpEnUS;
}

export function phone(value: string) {
  const digits = value.replace(/\D/g, "");
  if (digits.length === 10) {
    return digits.replace(/(\d{3})(\d{3})(\d{2})(\d{2})/, "$1 $2 $3 $4");
  }
  if (digits.length === 12 && digits.startsWith("90")) {
    return digits.replace(
      /(\d{2})(\d{3})(\d{3})(\d{2})(\d{2})/,
      "+$1 $2 $3 $4 $5",
    );
  }
  return value;
}

export function licensePlate(value: string) {
  return value.trim().toUpperCase().replace(/\s+/g, " ");
}

export function fileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function slug(value: string) {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

export function uuid() {
  return crypto.randomUUID();
}

export function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

export function toIso(value: Date) {
  return formatISO(value);
}
