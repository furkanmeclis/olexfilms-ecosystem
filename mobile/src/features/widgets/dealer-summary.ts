// Bayi widget'ları için saf özetler: API yanıtı → widget `props`.
// Saf dosya: çeviri, dil, saat ve logo yolu parametre gelir.
//
// Gizlilik: `Service`'ten yalnız `ServiceFields` okunur. Müşteri, organizasyon,
// not, şasi ve gelir tutarı tipte bile yoktur; hiçbir çıktıya geçemez.
import type { components } from '@/core/api/schema';

import { intlLocale } from '../../core/digits.ts';
import { BOARD_GROUPS, boardRows, uniqueCategories } from '../dealer-home/board.ts';
import type { AudienceInput } from './audience.ts';
import { pendingBlockedState, pendingRows, quickActions } from './audience.ts';
import type { TFn } from './format.ts';
import { count, maskedMoney, money, plateText, staleAgo, waitDays } from './format.ts';
import { links } from './links.ts';
import type {
  KpiBar,
  KpiCard,
  KpiSummary,
  PanomRow,
  PanomSummary,
  PendingRow,
  PendingSummary,
  QuickAction,
  QuickSummary,
  RevenueSummary,
} from './types.ts';

type Service = components['schemas']['Service'];
type Report = components['schemas']['ReportEnvelope'];

/** Okunmasına izin verilen alanlar. */
export type ServiceFields = Pick<
  Service,
  'uuid' | 'status' | 'car_brand' | 'car_model' | 'model_year' | 'plate' | 'plate_country' | 'item_categories' | 'created_at'
>;

export type ServicePage = { items: ServiceFields[]; total: number };

export type DealerCtx = {
  /** `widgets` namespace'i. */
  t: TFn;
  locale: string;
  now: Date;
  brandLogo: (brandUuid: string | undefined) => string | null;
};

/** Çubuk grafiğin en büyük yüksekliği (pt). */
export const MAX_BAR = 86;
const BAR_COUNT = 5;
const PANOM_ROWS = 4;

const KPI_CARDS = [
  { key: 'services_created', label: 'kpi.created' },
  { key: 'services_completed', label: 'kpi.completed' },
  { key: 'services_open', label: 'kpi.open' },
  { key: 'warranties_active', label: 'kpi.active_warranty' },
  { key: 'warranties_expiring', label: 'kpi.expiring' },
] as const;

const pad = (n: number) => String(n).padStart(2, '0');

/** Ayın ilk ve son günü (UTC, ikisi de dahil): API `from`/`to` UTC günüdür. */
export function monthRange(now: Date): { from: string; to: string } {
  const y = now.getUTCFullYear();
  const m = now.getUTCMonth();
  const last = new Date(Date.UTC(y, m + 1, 0)).getUTCDate();
  return { from: `${y}-${pad(m + 1)}-01`, to: `${y}-${pad(m + 1)}-${pad(last)}` };
}

/** Yerel günün başlangıcı (RFC3339, UTC): "bugün biten" için `completed_from`. */
export function localDayStartIso(now: Date): string {
  return new Date(now.getFullYear(), now.getMonth(), now.getDate()).toISOString();
}

function carTitle(s: Pick<ServiceFields, 'car_brand' | 'car_model'>, t: TFn) {
  const name = [s.car_brand?.name, s.car_model?.name].filter((n): n is string => !!n && n.trim() !== '').join(' ');
  return name || t('common.auto_vehicle');
}

function firstChar(s: string) {
  const c = Array.from(s.trim())[0];
  return c ? c.toLocaleUpperCase() : '';
}

function tone(status: string): PanomRow['statusTone'] {
  if (status === 'completed') return 'done';
  if (status === 'cancelled') return 'problem';
  if (status === 'draft') return 'neutral';
  return 'progress';
}

// SF Symbol adları; widget `Image(systemName:)` ile çizer.
const STATUS_ICONS: Record<string, string> = {
  draft: 'clock',
  pending: 'clock',
  processing: 'wrench.and.screwdriver',
  ready: 'checkmark.circle',
  completed: 'checkmark',
};

export function buildPanom(
  input: { next: ServicePage; active: ServicePage; done: ServicePage; canCreate: boolean },
  ctx: DealerCtx,
): PanomSummary {
  const { t, locale, now } = ctx;
  const title = t('panom.title');
  const totals = { next: input.next.total, active: input.active.total, done: input.done.total };
  const queue = boardRows(input.next.items, { ...BOARD_GROUPS[0], limit: Infinity });
  const wait = (s: ServiceFields) => staleAgo(now.getTime() - Date.parse(s.created_at), t);

  const rows: PanomRow[] = queue.slice(0, PANOM_ROWS).map((s) => ({
    uuid: s.uuid,
    url: links.bayi.service(s.uuid),
    carTitle: carTitle(s, t),
    plate: plateText(s.plate ?? ''),
    plateCountry: s.plate_country ?? '',
    meta: [uniqueCategories(s)[0], wait(s)].filter(Boolean).join(' · '),
    statusIcon: STATUS_ICONS[s.status] ?? 'clock',
    statusTone: tone(s.status),
    logoUri: ctx.brandLogo(s.car_brand?.uuid),
    initial: firstChar(s.car_brand?.name ?? carTitle(s, t)),
  }));

  const oldest = queue[0];
  const base = {
    title,
    next: count(totals.next, locale),
    active: count(totals.active, locale),
    done: count(totals.done, locale),
    rows,
    longest: oldest
      ? {
          uuid: oldest.uuid,
          url: links.bayi.service(oldest.uuid),
          carTitle: carTitle(oldest, t),
          plate: plateText(oldest.plate ?? ''),
          waitLabel: t('panom.longest_wait', { wait: wait(oldest) }),
        }
      : null,
  };
  if (totals.next + totals.active + totals.done === 0) {
    return {
      state: 'empty',
      card: {
        eyebrow: title,
        title: t('state.empty_queue_title'),
        body: t('state.empty_queue_body'),
        ...(input.canCreate ? { cta: t('state.new_service') } : {}),
      },
      ...base,
    };
  }
  return { state: 'ok', ...base };
}

const QUICK_URLS: Record<QuickAction['id'], () => string> = {
  scan: links.bayi.scan,
  new_service: links.bayi.newService,
  measure: links.bayi.measure,
  customers: links.bayi.customers,
  services: links.bayi.services,
};

/** `customersTotal`: bilinmiyorsa `null`/yok; sayı yalnız müşteriler düğmesi varsa gösterilir. */
export function buildQuick(audience: AudienceInput, ctx: DealerCtx, customersTotal?: number | null): QuickSummary {
  const { t } = ctx;
  const actions: QuickAction[] = quickActions(audience).map(({ id }) => ({ id, label: t(`quick.${id}`), url: QUICK_URLS[id]() }));
  const showCount = actions.some((a) => a.id === 'customers') && typeof customersTotal === 'number';
  return {
    state: 'ok',
    title: t('quick.title'),
    actions,
    customersCount: showCount ? t('quick.customers_count', { count: customersTotal }) : null,
  };
}

/**
 * Son 30 gün. `forbidden`: sunucu 403 döndü. Hiç bilinen kart yoksa da
 * `no_permission` (rol ve modül yalnız mevcut kartları bırakır).
 */
export function buildKpi(input: { overview: Report | null; trend: Report | null; forbidden?: boolean }, ctx: DealerCtx): KpiSummary {
  const { t, locale } = ctx;
  const title = t('kpi.title');
  const weeklyTitle = t('kpi.weekly_title');
  const points = input.forbidden
    ? []
    : (input.overview?.series.find((s) => s.key === 'cards')?.points ?? []);
  const byKey = new Map(points.map((p) => [p.key, p.value]));

  const cards: KpiCard[] = [];
  for (const def of KPI_CARDS) {
    const value = byKey.get(def.key);
    if (value !== undefined) cards.push({ key: def.key, label: t(def.label), value: count(value, locale) });
  }
  if (cards.length === 0) {
    return { state: 'no_permission', title, weeklyTitle, cards: [], primary: null, summary: null, bars: [] };
  }

  const completed = cards.find((c) => c.key === 'services_completed');
  const primary = completed ? { ...completed, label: t('kpi.completed_service') } : cards[0];
  const created = cards.find((c) => c.key === 'services_created');
  const open = cards.find((c) => c.key === 'services_open');

  const created5 = input.trend?.series.find((s) => s.key === 'created')?.points.slice(-BAR_COUNT) ?? [];
  const max = Math.max(0, ...created5.map((p) => p.value));
  const bars: KpiBar[] = created5.map((p, i) => ({
    label: i === created5.length - 1 ? t('kpi.this_week') : t('kpi.weeks_ago', { count: created5.length - 1 - i }),
    height: max > 0 ? Math.round((Math.max(0, p.value) / max) * MAX_BAR) : 0,
    current: i === created5.length - 1,
  }));

  return {
    state: 'ok',
    title,
    weeklyTitle,
    cards,
    primary,
    summary: created && open ? t('kpi.summary', { created: created.value, open: open.value }) : null,
    bars,
  };
}

/** `ready`: teslime hazır hizmetler (en eskisi için); `claims`, `expiring` null ise 0 sayılır. */
export function buildPending(
  input: { audience: AudienceInput; ready: ServicePage | null; claims: { total: number } | null; expiring: number | null },
  ctx: DealerCtx,
): PendingSummary {
  const { t, locale, now } = ctx;
  const title = t('pending.title');
  const ids = pendingRows(input.audience);
  if (ids.length === 0) return { state: pendingBlockedState(input.audience), title, rows: [] };

  const counts = { ready: input.ready?.total ?? 0, claim: input.claims?.total ?? 0, expiring: input.expiring ?? 0 };
  const oldest = [...(input.ready?.items ?? [])].sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at))[0];
  const readyDetail =
    counts.ready > 0 && oldest
      ? t('pending.ready_detail', { vehicle: carTitle(oldest, t), count: waitDays(oldest.created_at, now) })
      : '';

  const defs: Record<PendingRow['id'], Omit<PendingRow, 'id' | 'count'>> = {
    ready: { label: t('pending.ready'), detail: readyDetail, url: links.bayi.services() },
    claim: { label: t('pending.claim'), detail: t('pending.claim_detail'), url: links.bayi.claims() },
    expiring: { label: t('pending.expiring'), detail: t('pending.expiring_detail'), url: links.bayi.warranties() },
  };
  const rows: PendingRow[] = ids.map((id) => ({ id, ...defs[id], count: count(counts[id], locale) }));

  if (ids.every((id) => counts[id] === 0)) {
    return {
      state: 'empty',
      card: { eyebrow: title, title: t('state.empty_queue_title'), body: t('state.empty_queue_body') },
      title,
      rows,
    };
  }
  return { state: 'ok', title, rows };
}

/** Ay geliri; tutar varsayılan gizli (`revealed: false`). */
export function buildRevenue(
  input: { currency: string; totals: { income: string; expense: string; net: string } },
  ctx: DealerCtx,
): RevenueSummary {
  const { t, locale, now } = ctx;
  const month = new Intl.DateTimeFormat(intlLocale(locale), { month: 'long', timeZone: 'UTC' }).format(now);
  return {
    state: 'ok',
    title: t('revenue.title'),
    monthTitle: t('revenue.month_income', { month }),
    currency: input.currency,
    incomeLabel: money(input.totals.income, input.currency, locale),
    netLabel: money(input.totals.net, input.currency, locale),
    expenseLabel: money(input.totals.expense, input.currency, locale),
    maskedLabel: maskedMoney(input.currency, locale),
    netTitle: t('revenue.net'),
    expenseTitle: t('revenue.expense'),
    hiddenTitle: t('revenue.hidden'),
    tapToReveal: t('revenue.tap_to_reveal'),
    revealed: false,
  };
}
