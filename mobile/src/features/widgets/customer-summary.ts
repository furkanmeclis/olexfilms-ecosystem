// Müşteri widget'ları için saf özetler: API yanıtı → widget `props`.
// Saf dosya: çeviri (`t`, `tc`), dil ve görsel yolları parametre gelir.
import type { components } from '@/core/api/schema';

import { intlLocale } from '../../core/digits.ts';
import { MAX_VEHICLES } from './config.ts';
import type { TFn } from './format.ts';
import { plateText, timeLeft, timeLeftShort } from './format.ts';
import { links } from './links.ts';
import type {
  HizmetService,
  HizmetStep,
  HizmetSummary,
  HizmetVehicle,
  KartSummary,
  KorumaLocked,
  KorumaSummary,
  KorumaVehicle,
  WidgetCard,
} from './types.ts';

type PortalVehicleDetail = components['schemas']['PortalVehicleDetail'];
type PortalActiveWarranty = components['schemas']['PortalActiveWarranty'];
type PortalServiceListItem = components['schemas']['PortalServiceListItem'];
type PortalService = components['schemas']['PortalService'];

/** Hizmet seçimi iki kaynaktan da beslenir: araç ayrıntısındaki liste ya da tek hizmet. */
export type ServiceLike = PortalServiceListItem | PortalService;

export type CustomerAssets = {
  /** Araç kapağı (`file://`); yoksa `null`. */
  hero: (vehicleUuid: string) => string | null;
  brandLogo: (brandUuid: string | undefined) => string | null;
  qr: (publicCode: string) => string | null;
};

export type CustomerCtx = {
  /** `widgets` namespace'i. */
  t: TFn;
  /** `customer` namespace'i (`timeLeft` için). */
  tc: TFn;
  locale: string;
  assets: CustomerAssets;
};

export type HizmetInput = { vehicle: PortalVehicleDetail; services?: ServiceLike[] };

const MAX_WARRANTIES = 3;
const WARN_BELOW_PERCENT = 15;
const STEP_KEYS = ['pending', 'processing', 'ready', 'completed'] as const;
const OPEN_STATUSES = ['draft', 'pending', 'processing', 'ready'];

const byUuid = (a: { uuid: string }, b: { uuid: string }) => (a.uuid < b.uuid ? -1 : a.uuid > b.uuid ? 1 : 0);
const clampPercent = (n: number) => Math.min(100, Math.max(0, n));

function time(iso: string | null | undefined) {
  const t = iso ? Date.parse(iso) : NaN;
  return Number.isNaN(t) ? 0 : t;
}

function formatDate(iso: string | null | undefined, locale: string) {
  if (!iso || Number.isNaN(Date.parse(iso))) return '';
  return new Intl.DateTimeFormat(intlLocale(locale), { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(iso));
}

function firstChar(s: string) {
  const c = Array.from(s.trim())[0];
  return c ? c.toLocaleUpperCase() : '';
}

type Prepared = {
  detail: PortalVehicleDetail;
  title: string;
  /** Aktif, artan `days_left`. */
  warranties: PortalActiveWarranty[];
};

function titleOf(v: PortalVehicleDetail, t: TFn) {
  const name = [v.car_brand?.name, v.car_model?.name].filter((s): s is string => !!s && s.trim() !== '').join(' ');
  return name || t('common.auto_vehicle');
}

/**
 * En kısa süreli garanti taşıyan araç önce, garantisiz sonda, `uuid` eşitlik
 * bozar; `MAX_VEHICLES` ile kesilir (uzun filolarda seçenek listesi kısalır).
 * Süresi bitmiş (`days_left <= 0`) garanti aktif sayılmaz.
 */
function prepare(vehicles: PortalVehicleDetail[], t: TFn): Prepared[] {
  const prepared = vehicles.map((detail) => ({
    detail,
    title: titleOf(detail, t),
    warranties: detail.active_warranties.filter((w) => w.days_left > 0).sort((a, b) => a.days_left - b.days_left || byUuid(a, b)),
  }));
  const shortest = (p: Prepared) => p.warranties[0]?.days_left;
  prepared.sort((a, b) => {
    const da = shortest(a);
    const db = shortest(b);
    if (da !== undefined && db !== undefined && da !== db) return da - db;
    if (da === undefined && db !== undefined) return 1;
    if (da !== undefined && db === undefined) return -1;
    return byUuid(a.detail, b.detail);
  });
  return prepared.slice(0, MAX_VEHICLES);
}

function identity(p: Prepared, ctx: CustomerCtx) {
  const v = p.detail;
  return {
    id: v.uuid,
    url: links.vehicle(v.uuid),
    title: p.title,
    plate: plateText(v.plate ?? ''),
    plateCountry: v.plate_country ?? '',
    heroUri: ctx.assets.hero(v.uuid),
  };
}

function brandOf(p: Prepared, ctx: CustomerCtx) {
  return {
    brandLogoUri: ctx.assets.brandLogo(p.detail.car_brand?.uuid),
    brandInitial: firstChar(p.detail.car_brand?.name ?? p.title),
  };
}

function noVehicleCard(eyebrow: string, t: TFn): WidgetCard {
  return { eyebrow, title: t('state.no_vehicle_title'), body: t('state.no_vehicle_body') };
}

/** En kısa süreli garantiye göre kilit ekranı metinleri. */
function lockedOf(p: Prepared, ctx: CustomerCtx): KorumaLocked {
  const { t, tc } = ctx;
  const title = t('koruma.eyebrow');
  const shortest = p.warranties[0];
  if (!shortest) {
    const none = t('state.no_warranty_title');
    return { title, short: '', long: none, inline: none };
  }
  const time = timeLeft(shortest.days_left, tc);
  return {
    title,
    short: timeLeftShort(shortest.days_left, t),
    long: t('koruma.left_long', { time }),
    inline: t('koruma.lock_inline', { time }),
  };
}

export function buildKoruma(vehicles: PortalVehicleDetail[], ctx: CustomerCtx): KorumaSummary {
  const { t, tc, locale } = ctx;
  const eyebrow = t('koruma.eyebrow');
  const leftCaption = t('koruma.left_caption');
  const prepared = prepare(vehicles, t);
  if (prepared.length === 0) {
    return { state: 'empty', card: noVehicleCard(eyebrow, t), eyebrow, leftCaption, vehicles: [] };
  }
  const out: KorumaVehicle[] = prepared.map((p) => {
    const lastService = formatDate(p.detail.last_service_at ?? p.detail.service_summary.last_service_at, locale);
    return {
      ...identity(p, ctx),
      ...brandOf(p, ctx),
      activeCountLabel: t('koruma.active_count', { count: p.warranties.length }),
      lastServiceLabel: lastService ? t('koruma.last_service', { date: lastService }) : '',
      warranties: p.warranties.slice(0, MAX_WARRANTIES).map((w) => ({
        name: w.product.name,
        leftLabel: timeLeft(w.days_left, tc),
        leftLongLabel: t('koruma.left_long', { time: timeLeft(w.days_left, tc) }),
        leftShortLabel: timeLeftShort(w.days_left, t),
        percent: clampPercent(w.percent_left),
        warn: w.percent_left < WARN_BELOW_PERCENT,
      })),
      emptyCard:
        p.warranties.length === 0
          ? { eyebrow: p.title, title: t('state.no_warranty_title'), body: t('state.no_warranty_body') }
          : null,
      locked: lockedOf(p, ctx),
    };
  });
  return { state: 'ok', eyebrow, leftCaption, vehicles: out };
}

/** Her garantili araç için en kısa süreli garanti kartı. */
export function buildKart(vehicles: PortalVehicleDetail[], ctx: CustomerCtx): KartSummary {
  const { t, locale } = ctx;
  const title = t('card.title');
  const prepared = prepare(vehicles, t);
  if (prepared.length === 0) return { state: 'empty', card: noVehicleCard(title, t), title, vehicles: [] };
  const withWarranty = prepared.filter((p) => p.warranties.length > 0);
  if (withWarranty.length === 0) {
    return {
      state: 'empty',
      card: { eyebrow: prepared[0].title, title: t('state.no_warranty_title'), body: t('state.no_warranty_body') },
      title,
      vehicles: [],
    };
  }
  return {
    state: 'ok',
    title,
    vehicles: withWarranty.map((p) => {
      const w = p.warranties[0];
      const { id, url, title: carTitle, plate, heroUri } = identity(p, ctx);
      return {
        id,
        url,
        publicCode: w.public_code,
        productName: w.product.name,
        untilLabel: t('card.until', { date: formatDate(w.end_at, locale) }),
        qrUri: ctx.assets.qr(w.public_code),
        heroUri,
        title: carTitle,
        plate,
      };
    }),
  };
}

function dealerOf(s: ServiceLike) {
  return 'dealer' in s ? s.dealer.name : s.organization.name;
}

/**
 * Gösterilecek hizmet: `pending/processing/ready` (müşteride `draft` da
 * beklemede) varsa en yenisi; yoksa en yeni `completed`; yoksa ya da en son
 * kayıt `cancelled` ise `null`.
 */
function pickService(services: ServiceLike[]): ServiceLike | null {
  const newest = (list: ServiceLike[], at: (s: ServiceLike) => number) =>
    [...list].sort((a, b) => at(b) - at(a) || byUuid(a, b))[0] ?? null;
  const created = (s: ServiceLike) => time(s.created_at);
  const open = newest(services.filter((s) => OPEN_STATUSES.includes(s.status)), created);
  if (open) return open;
  const latest = newest(services, created);
  if (!latest || latest.status === 'cancelled') return null;
  return newest(services.filter((s) => s.status === 'completed'), (s) => time(s.completed_at ?? s.created_at));
}

function stepsOf(status: string, t: TFn): HizmetStep[] {
  const index = status === 'completed' ? STEP_KEYS.length : Math.max(0, STEP_KEYS.indexOf(status === 'draft' ? 'pending' : (status as (typeof STEP_KEYS)[number])));
  return STEP_KEYS.map((key, i) => ({
    label: t(`service.steps.${key}`),
    phase: i < index ? 'done' : i === index ? 'current' : 'todo',
  }));
}

export function buildHizmet(inputs: HizmetInput[], ctx: CustomerCtx): HizmetSummary {
  const { t } = ctx;
  const eyebrow = t('koruma.eyebrow');
  const sources = new Map(inputs.map((i) => [i.vehicle.uuid, i.services ?? i.vehicle.services]));
  const prepared = prepare(inputs.map((i) => i.vehicle), t);
  if (prepared.length === 0) return { state: 'empty', card: noVehicleCard(eyebrow, t), eyebrow, vehicles: [] };
  const vehicles: HizmetVehicle[] = prepared.map((p) => {
    const picked = pickService(sources.get(p.detail.uuid) ?? []);
    let service: HizmetService | null = null;
    if (picked) {
      const steps = stepsOf(picked.status, t);
      const done = picked.status === 'completed';
      service = {
        uuid: picked.uuid,
        url: links.service(picked.uuid),
        serviceNo: picked.service_no,
        dealerName: dealerOf(picked),
        statusLabel: (steps.find((s) => s.phase === 'current') ?? steps[steps.length - 1]).label,
        done,
        steps,
      };
    }
    return {
      ...identity(p, ctx),
      ...brandOf(p, ctx),
      service,
      emptyCard: service ? null : { eyebrow: p.title, title: t('service.no_open_title'), body: t('service.no_open_body') },
    };
  });
  return { state: 'ok', eyebrow, vehicles };
}
