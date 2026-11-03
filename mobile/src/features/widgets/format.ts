// Widget metin ve sayı biçimlendirmesi. Saf dosya: `t` ve `locale` parametre gelir.
import { intlLocale, toLatinDigits } from '../../core/digits.ts';
import { splitTimeLeft } from '../customer-space/time-left.ts';

export type TFn = (key: string, options?: Record<string, unknown>) => string;

const MASK = '•••••';

/**
 * Kalan süre, uzun metin. `t` `customer` namespace'inin çevirisidir
 * (`left_*` anahtarları), `home-parts.tsx` `useTimeLeft` ile aynı mantık.
 */
export function timeLeft(days: number, t: TFn): string {
  if (days <= 0) return t('left_ended');
  const { years, months } = splitTimeLeft(days);
  if (years > 0 && months > 0) return t('left_years_months', { years, months });
  if (years > 0) return t('left_years', { count: years });
  if (months > 0) return t('left_months', { count: months });
  return t('left_days', { count: days });
}

/** Kilit dairesi için kısa süre ("2y 4a"). `t` `widgets` namespace'inin çevirisidir. */
export function timeLeftShort(days: number, t: TFn): string {
  const { years, months } = days > 0 ? splitTimeLeft(days) : { years: 0, months: 0 };
  if (years > 0 && months > 0) return t('koruma.left_compact_years_months', { years, months });
  if (years > 0) return t('koruma.left_compact_years', { count: years });
  if (months > 0) return t('koruma.left_compact_months', { count: months });
  return t('koruma.left_compact_days', { count: Math.max(0, days) });
}

/** Gruplu sayı ("1.248"), dilin rakamlarıyla. */
export function count(n: number, locale: string): string {
  return new Intl.NumberFormat(intlLocale(locale)).format(n);
}

/** `AccountingAmount` metni, kuruşsuz para birimi. Okunamayan tutar 0 sayılır. */
export function money(amountStr: string, currency: string, locale: string): string {
  const parsed = Number(amountStr.trim() === '' ? NaN : amountStr);
  const value = Number.isFinite(parsed) ? parsed : 0;
  try {
    return new Intl.NumberFormat(intlLocale(locale), {
      style: 'currency',
      currency,
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(value);
  } catch {
    return `${currency} ${count(Math.round(value), locale)}`;
  }
}

/** Gizli ciro: para birimi sembolü ve nokta, tutar bilgisi yok ("₺•••••"). */
export function maskedMoney(currency: string, locale: string): string {
  try {
    return new Intl.NumberFormat(intlLocale(locale), {
      style: 'currency',
      currency,
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    })
      .formatToParts(0)
      .map((p) => (p.type === 'integer' ? MASK : p.value))
      .join('');
  } catch {
    return `${currency} ${MASK}`;
  }
}

/** "Güncellendi 14:32" metnindeki saat. */
export function updatedAt(date: Date, t: TFn, locale: string): string {
  const time = new Intl.DateTimeFormat(intlLocale(locale), { hour: '2-digit', minute: '2-digit' }).format(date);
  return t('common.updated_at', { time });
}

/** Verinin yaşı, kısa süre ("2 saat"); `state.stale_body` içine girer. */
export function staleAgo(ms: number, t: TFn): string {
  const minutes = Math.floor(Math.max(0, ms) / 60_000);
  if (minutes < 60) return t('common.minutes_count', { count: Math.max(1, minutes) });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t('common.hours_count', { count: hours });
  return t('common.days_count', { count: Math.floor(hours / 24) });
}

const DAY_MS = 86_400_000;

function localDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** `createdAt`'ten `now`'a yerel takvim günü farkı; gelecek veya geçersiz tarih 0. */
export function waitDays(createdAt: string | Date, now: Date): number {
  const created = typeof createdAt === 'string' ? new Date(createdAt) : createdAt;
  if (Number.isNaN(created.getTime())) return 0;
  return Math.max(0, Math.round((localDay(now) - localDay(created)) / DAY_MS));
}

/** Plaka her dilde Latin ve LTR; yazılmış Arap rakamları Latin'e çevrilir. */
export function plateText(plate: string): string {
  return toLatinDigits(plate);
}
