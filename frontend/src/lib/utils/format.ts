import {
  format as formatDateFns,
  formatDistanceToNow,
  formatISO,
  parseISO,
  type Locale as DateFnsLocale,
} from "date-fns";
import { enUS as dateFnsEnUS, tr as dateFnsTr } from "date-fns/locale";
import {
  enUS as dayPickerEnUS,
  tr as dayPickerTr,
} from "react-day-picker/locale";

import { i18nConfig, type AppLocale } from "@/config/i18n";

export function dateFnsLocale(
  locale: AppLocale = i18nConfig.defaultLocale,
): DateFnsLocale {
  return locale === "tr" ? dateFnsTr : dateFnsEnUS;
}

/** DayPicker locale (`react-day-picker/locale`) for calendar UI. */
export function dayPickerLocale(locale: AppLocale = i18nConfig.defaultLocale) {
  return locale === "tr" ? dayPickerTr : dayPickerEnUS;
}

function dateLocale(locale: AppLocale = i18nConfig.defaultLocale) {
  return dateFnsLocale(locale);
}

export function currency(
  value: number,
  currencyCode = "TRY",
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "currency",
    currency: currencyCode,
  }).format(value);
}

export function money(
  value: number,
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

export function date(
  value: Date | string,
  pattern = "dd.MM.yyyy",
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  return formatDateFns(d, pattern, { locale: dateLocale(locale) });
}

export function datetime(
  value: Date | string,
  pattern = "dd.MM.yyyy HH:mm",
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  return formatDateFns(d, pattern, { locale: dateLocale(locale) });
}

export function relativeDatetime(
  value: Date | string,
  locale: AppLocale = i18nConfig.defaultLocale,
) {
  const d = typeof value === "string" ? parseISO(value) : value;
  return formatDistanceToNow(d, {
    addSuffix: true,
    locale: dateLocale(locale),
  });
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
