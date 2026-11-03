// Widget verisi için uç çağrıları. Token akışı `core/api/client`tedir; widget token görmez.
// Her çağrı sonucu üç haldir: veri, izin/modül engeli ya da hata (önceki içerik kalır).
import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { BOARD_GROUPS, statusParam } from '@/features/dealer-home/board';

import type { ServicePage } from './dealer-summary';
import { localDayStartIso, monthRange } from './dealer-summary';
import { MAX_VEHICLES } from './config';
import type { WidgetState } from './types';

type PortalVehicleDetail = components['schemas']['PortalVehicleDetail'];
type PortalServiceListItem = components['schemas']['PortalServiceListItem'];
type Report = components['schemas']['ReportEnvelope'];

export type Fetched<T> =
  | { status: 'ok'; value: T }
  | { status: 'blocked'; state: Extract<WidgetState, 'no_permission' | 'module_off'> }
  | { status: 'failed' };

/** 403 `FEATURE_DISABLED` modül kapalı, diğer 403 izin yok; geri kalanı (ağ, 5xx, 401) hata. */
function classify<T>(result: PromiseSettledResult<T>): Fetched<T> {
  if (result.status === 'fulfilled') return { status: 'ok', value: result.value };
  const e = result.reason;
  if (isApiError(e) && e.status === 403) {
    return { status: 'blocked', state: e.code === 'FEATURE_DISABLED' ? 'module_off' : 'no_permission' };
  }
  return { status: 'failed' };
}

async function attempt<T>(call: () => Promise<T>): Promise<Fetched<T>> {
  const [result] = await Promise.allSettled([call()]);
  return classify(result);
}

// --- Müşteri ---

export type CustomerData = {
  vehicles: PortalVehicleDetail[];
  /** Açık hizmetler ile son hizmetlerin birleşimi (`uuid` tekil). */
  services: PortalServiceListItem[];
};

/** Hepsi ya da hiç: kısmi veri eski içeriği yarım yamalak ezmesin diye bir hata tüm müşteri widget'larını atlatır. */
export async function fetchCustomer(): Promise<CustomerData | null> {
  const [list, open, latest] = await Promise.allSettled([
    unwrap(api.GET('/v1/portal/vehicles', { params: { query: { limit: MAX_VEHICLES } } })),
    unwrap(
      api.GET('/v1/portal/services', {
        params: { query: { status: 'pending,processing,ready', sort: '-created_at', limit: 20 } },
      }),
    ),
    unwrap(api.GET('/v1/portal/services', { params: { query: { sort: '-created_at', limit: 20 } } })),
  ]);
  if (list.status !== 'fulfilled' || open.status !== 'fulfilled' || latest.status !== 'fulfilled') return null;

  const items = list.value.items;
  const wanted = items.filter((v) => v.active_warranty_count > 0 || v.service_count > 0);
  // Hiçbirinde garanti ya da hizmet yoksa da araçlar boş duruma düşmesin diye hepsi çekilir.
  const targets = wanted.length > 0 ? wanted : items;
  const details = await Promise.allSettled(
    targets.map((v) => unwrap(api.GET('/v1/portal/vehicles/{uuid}', { params: { path: { uuid: v.uuid } } }))),
  );
  if (details.some((d) => d.status !== 'fulfilled')) return null;

  const services = new Map<string, PortalServiceListItem>();
  for (const s of [...open.value.items, ...latest.value.items]) services.set(s.uuid, s);
  return {
    vehicles: details.map((d) => (d as PromiseFulfilledResult<PortalVehicleDetail>).value),
    services: [...services.values()],
  };
}

// --- Bayi ---

/** Etkin modüller; okunamazsa `null` (hepsi açık sayılır, `core/modules.ts` ile aynı). */
export async function fetchEnabledModules(): Promise<Set<string> | null> {
  const r = await attempt(() => unwrap(api.GET('/v1/features')));
  return r.status === 'ok' ? new Set<string>(r.value.enabled) : null;
}

/** Panom üç panosu; satırlar `next`ten, `done` yalnız yerel gün başından beri biten sayısı için. */
export function fetchBoard(now: Date): Promise<Fetched<{ next: ServicePage; active: ServicePage; done: ServicePage }>> {
  const [next, active, done] = BOARD_GROUPS;
  const page = (g: (typeof BOARD_GROUPS)[number], extra: { completed_from?: string }, limit: number) =>
    unwrap(api.GET('/v1/services', { params: { query: { status: statusParam(g), sort: g.sort, limit, offset: 0, ...extra } } }));
  return attempt(async () => {
    const [n, a, d] = await Promise.all([
      page(next, {}, 4),
      page(active, {}, 4),
      page(done, { completed_from: localDayStartIso(now) }, 1),
    ]);
    return { next: n, active: a, done: d };
  });
}

export function fetchCustomersTotal(): Promise<Fetched<number>> {
  return attempt(async () => (await unwrap(api.GET('/v1/customers', { params: { query: { limit: 1 } } }))).total);
}

export function fetchOverview(): Promise<Fetched<Report>> {
  return attempt(() => unwrap(api.GET('/v1/reports/overview', { params: { query: { period: '30d' } } })));
}

export function fetchTrend(): Promise<Fetched<Report>> {
  return attempt(() =>
    unwrap(
      api.GET('/v1/reports/{report}', {
        params: { path: { report: 'services.trend' }, query: { period: '30d', granularity: 'week' } },
      }),
    ),
  );
}

/** Teslime hazır hizmetler: en eskisi ve toplam. */
export function fetchReady(): Promise<Fetched<ServicePage>> {
  return attempt(() =>
    unwrap(api.GET('/v1/services', { params: { query: { status: 'ready', sort: 'created_at', limit: 1, offset: 0 } } })),
  );
}

export function fetchOpenClaims(): Promise<Fetched<{ total: number }>> {
  return attempt(async () => ({
    total: (await unwrap(api.GET('/v1/warranty-claims', { params: { query: { status: 'open,dealer_review', limit: 1 } } }))).total,
  }));
}

export function fetchPnl(now: Date) {
  return attempt(async () => {
    const { from, to } = monthRange(now);
    const r = await unwrap(api.GET('/v1/accounting/reports/pnl', { params: { query: { from, to } } }));
    return { currency: r.currency, totals: r.totals };
  });
}
