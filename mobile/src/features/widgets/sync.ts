// Widget eşitlemesi: veri çek, görselleri yaz, seçenekleri yaz, zaman çizelgesini yaz.
// Native modüle yalnız `layouts` kaydı ve (kayıt doluysa) `images` üzerinden erişir.
import type { WidgetConfigurationEnum } from 'expo-widgets';

import type { components } from '@/core/api/schema';
import type { MobileMe } from '@/core/auth/api';
import { i18n, isRTL } from '@/core/i18n';
import { kindsOf } from '@/core/spaces';
import { publicWarrantyUrl } from '@/features/services/lib';

import type { AudienceInput } from './audience';
import { audienceOf, can, pendingRows, quickActions, resolveState } from './audience';
import { wipeWidgets } from './clear';
import type { WidgetName } from './config';
import { CUSTOMER_WIDGETS, DEALER_WIDGETS } from './config';
import type { CustomerAssets, CustomerCtx, HizmetInput, ServiceLike } from './customer-summary';
import { buildHizmet, buildKart, buildKoruma } from './customer-summary';
import type { DealerCtx } from './dealer-summary';
import { buildKpi, buildPanom, buildPending, buildQuick, buildRevenue } from './dealer-summary';
import type { Fetched } from './fetch';
import {
  fetchBoard,
  fetchCustomer,
  fetchCustomersTotal,
  fetchEnabledModules,
  fetchOpenClaims,
  fetchOverview,
  fetchPnl,
  fetchReady,
  fetchTrend,
} from './fetch';
import { updatedAt } from './format';
import type { WidgetHandle } from './layouts';
import { widgetFor, widgetsAvailable } from './layouts';
import { links } from './links';
import { signedOutTitleKey } from './signed-out';
import type { SyncTicket } from './sync-gate';
import { saveSyncRecord, syncGate } from './sync-state';
import { translator } from './translators';
import type { Timeline } from './timeline';
import { buildEntries, shiftWarranties, staleLabelFor } from './timeline';
import { WIDGET_THEME } from './theme';
import type { BaseProps, WidgetCard, WidgetState } from './types';

type Images = typeof import('./images');
type Option = WidgetConfigurationEnum;
type PortalVehicleDetail = components['schemas']['PortalVehicleDetail'];

/** Bir widget'a yazılacak: seçenekler (`null`: sıfırla) sonra içerik. */
type Write = { name: WidgetName; options?: Option[] | null; commit: (widget: WidgetHandle) => void };

type Env = {
  now: Date;
  lang: string;
  rtl: boolean;
  t: ReturnType<typeof translator>;
  tc: ReturnType<typeof translator>;
  images: Images | null;
  marks: { markUri: string; markMonoUri: string };
  audience: AudienceInput;
  /** Bu çalıştırmada yazılan görsel dosya adları; `reconcile` bunları tutar. */
  keep: Set<string>;
};

type Plan = { writes: Write[]; failed: boolean };

const fileName = (uri: string) => uri.slice(uri.lastIndexOf('/') + 1);

/** Ortak alanlar; `state` ve `card` özetten gelir. */
function baseFor(env: Env, url: string): Omit<BaseProps, 'state' | 'card'> {
  return {
    rtl: env.rtl,
    theme: WIDGET_THEME,
    markUri: env.marks.markUri,
    markMonoUri: env.marks.markMonoUri,
    url,
    updatedLabel: updatedAt(env.now, env.t, env.lang),
    staleLabel: null,
  };
}

const WIDGET_URLS: Record<WidgetName, () => string> = {
  OlexKoruma: () => links.home('musteri'),
  OlexGarantiKarti: () => links.home('musteri'),
  OlexHizmetDurumu: () => links.home('musteri'),
  OlexPanom: links.bayi.services,
  OlexHizliIslem: links.bayi.home,
  OlexSon30Gun: links.bayi.reports,
  OlexBekleyenler: links.bayi.home,
  OlexCiro: links.bayi.accounting,
};

function stateCard(env: Env, name: WidgetName, state: WidgetState): WidgetCard {
  const eyebrow = env.t(signedOutTitleKey(name));
  if (state === 'no_permission') {
    return { eyebrow, title: env.t('state.no_permission_title'), body: env.t('state.no_permission_body') };
  }
  if (state === 'module_off') {
    // Yalnız talepler için ayrı bir açıklama var.
    return {
      eyebrow,
      title: env.t('state.module_off_title'),
      ...(name === 'OlexBekleyenler' ? { body: env.t('state.module_off_body_claims') } : {}),
    };
  }
  return {
    eyebrow,
    title: env.t('state.wrong_role_title'),
    body: env.t(CUSTOMER_WIDGETS.includes(name) ? 'state.wrong_role_customer_body' : 'state.wrong_role_dealer_body'),
  };
}

/** Özet kartsız bir engel durumuna düştüyse kart ekler. */
function withCard<S extends { state: WidgetState; card?: WidgetCard }>(env: Env, name: WidgetName, summary: S): S {
  const blocked: WidgetState[] = ['no_permission', 'module_off', 'wrong_role'];
  if (summary.card || !blocked.includes(summary.state)) return summary;
  return { ...summary, card: stateCard(env, name, summary.state) };
}

/** Veri çekmeden yazılan engel durumu (rol, izin, modül). */
function stateWrite(env: Env, name: WidgetName, state: WidgetState): Write {
  const props: BaseProps = { ...baseFor(env, WIDGET_URLS[name]()), state, card: stateCard(env, name, state) };
  return {
    name,
    options: CUSTOMER_WIDGETS.includes(name) ? null : undefined,
    commit: (w) => w.updateSnapshot(props),
  };
}

function timelineWrite<P extends object>(name: WidgetName, timeline: Timeline<P>, options?: Option[]): Write {
  return {
    name,
    options,
    commit: (w) => (timeline.mode === 'snapshot' ? w.updateSnapshot(timeline.props) : w.updateTimeline(timeline.entries)),
  };
}

const firstProps = <P extends object>(timeline: Timeline<P>): P =>
  timeline.mode === 'snapshot' ? timeline.props : timeline.entries[0].props;

/** İlk seçenek otomatiktir; ad `plaka · araç`. */
function vehicleOptions(env: Env, vehicles: { id: string; plate: string; title: string }[]): Option[] {
  return [
    { name: env.t('common.auto'), value: 'auto' },
    ...vehicles.map((v) => ({ name: [v.plate, v.title].filter(Boolean).join(' · '), value: v.id })),
  ];
}

// --- Müşteri ---

async function customerAssets(env: Env, vehicles: PortalVehicleDetail[]): Promise<CustomerAssets> {
  const { images, keep } = env;
  const heroes = new Map<string, string | null>();
  const logos = new Map<string, string | null>();
  const qrs = new Map<string, string | null>();
  const remember = (uri: string | null) => {
    if (uri) keep.add(fileName(uri));
    return uri;
  };
  if (images) {
    // Aynı model/marka tek dosyaya yazılır: art arda çağrı ikincide taze dosyayı bulur.
    for (const v of vehicles) heroes.set(v.uuid, remember(await images.storeHero(v)));
    for (const v of vehicles) {
      const brand = v.car_brand?.uuid;
      if (brand && !logos.has(brand)) logos.set(brand, remember(await images.storeBrandLogo(brand)));
    }
  }
  return {
    hero: (vehicleUuid) => heroes.get(vehicleUuid) ?? null,
    brandLogo: (brandUuid) => (brandUuid ? (logos.get(brandUuid) ?? null) : null),
    qr: (code) => {
      if (!images) return null;
      if (!qrs.has(code)) qrs.set(code, remember(images.storeQr(code, publicWarrantyUrl(code))));
      return qrs.get(code) ?? null;
    },
  };
}

async function planCustomer(env: Env): Promise<Plan> {
  const writes = DEALER_WIDGETS.map((name) => stateWrite(env, name, 'wrong_role'));
  const data = await fetchCustomer();
  if (!data) return { writes, failed: true };

  const ctx: CustomerCtx = { t: env.t, tc: env.tc, locale: env.lang, assets: await customerAssets(env, data.vehicles) };
  const stale = staleLabelFor(env.t);
  const base = baseFor(env, links.home('musteri'));

  // Kalan gün her girdi için o ana göre hesaplanır; biten garanti düşer.
  const koruma = buildEntries('OlexKoruma', env.now, (at) => ({ ...base, ...buildKoruma(shiftWarranties(data.vehicles, env.now, at), ctx) }), stale);
  const kart = buildEntries('OlexGarantiKarti', env.now, (at) => ({ ...base, ...buildKart(shiftWarranties(data.vehicles, env.now, at), ctx) }), stale);

  // Araç ayrıntısındaki son hizmetler eski kalmış olabilir; liste daha taze olduğu için üstüne yazar.
  const inputs: HizmetInput[] = data.vehicles.map((vehicle) => {
    const merged = new Map<string, ServiceLike>(vehicle.services.map((s) => [s.uuid, s]));
    for (const s of data.services) if (s.vehicle_uuid === vehicle.uuid) merged.set(s.uuid, s);
    return { vehicle, services: [...merged.values()] };
  });
  const hizmet = buildEntries('OlexHizmetDurumu', env.now, () => ({ ...base, ...buildHizmet(inputs, ctx) }), stale);

  writes.push(
    timelineWrite('OlexKoruma', koruma, vehicleOptions(env, firstProps(koruma).vehicles)),
    timelineWrite('OlexGarantiKarti', kart, vehicleOptions(env, firstProps(kart).vehicles)),
    timelineWrite('OlexHizmetDurumu', hizmet, vehicleOptions(env, firstProps(hizmet).vehicles)),
  );
  return { writes, failed: false };
}

// --- Bayi ---

const valueOf = <T>(r: Fetched<T> | null): T | null => (r?.status === 'ok' ? r.value : null);

async function planDealer(env: Env): Promise<Plan> {
  const { audience, now } = env;
  const writes = CUSTOMER_WIDGETS.map((name) => stateWrite(env, name, 'wrong_role'));
  let failed = false;
  const stale = staleLabelFor(env.t);

  const panomState = resolveState('OlexPanom', audience);
  const ciroState = resolveState('OlexCiro', audience);
  const rows = pendingRows(audience);
  const showCustomers = quickActions(audience).some((a) => a.id === 'customers');

  const [board, customers, overview, trend, ready, claims, pnl] = await Promise.all([
    panomState === 'ok' ? fetchBoard(now) : null,
    showCustomers ? fetchCustomersTotal() : null,
    // Son 30 gün her zaman sorar (tek yetki kaynağı sonuçtur); Bekleyenler'in bitecek garanti sayısı da buradan gelir.
    fetchOverview(),
    fetchTrend(),
    rows.includes('ready') ? fetchReady() : null,
    rows.includes('claim') ? fetchOpenClaims() : null,
    ciroState === 'ok' ? fetchPnl(now) : null,
  ]);

  // Panom
  if (panomState !== 'ok') writes.push(stateWrite(env, 'OlexPanom', panomState));
  else if (board?.status === 'blocked') writes.push(stateWrite(env, 'OlexPanom', board.state));
  else if (board?.status === 'ok') {
    const logos = new Map<string, string | null>();
    if (env.images) {
      for (const s of board.value.next.items) {
        const brand = s.car_brand?.uuid;
        if (brand && !logos.has(brand)) {
          const uri = await env.images.storeBrandLogo(brand);
          if (uri) env.keep.add(fileName(uri));
          logos.set(brand, uri);
        }
      }
    }
    const ctxAt = (at: Date): DealerCtx => ({
      t: env.t,
      locale: env.lang,
      now: at,
      brandLogo: (uuid) => (uuid ? (logos.get(uuid) ?? null) : null),
    });
    const input = { ...board.value, canCreate: can(audience, 'services.write') };
    const base = baseFor(env, WIDGET_URLS.OlexPanom());
    writes.push(
      timelineWrite('OlexPanom', buildEntries('OlexPanom', now, (at) => ({ ...base, ...buildPanom(input, ctxAt(at)) }), stale)),
    );
  } else failed = true;

  // Hızlı işlem: müşteri sayısı okunamazsa düğmeler yine çizilir.
  {
    const base = baseFor(env, WIDGET_URLS.OlexHizliIslem());
    const total = valueOf(customers);
    writes.push(
      timelineWrite(
        'OlexHizliIslem',
        buildEntries('OlexHizliIslem', now, (at) => ({ ...base, ...buildQuick(audience, { t: env.t, locale: env.lang, now: at, brandLogo: () => null }, total) }), stale),
      ),
    );
  }

  // Son 30 gün
  {
    const dctx = (at: Date): DealerCtx => ({ t: env.t, locale: env.lang, now: at, brandLogo: () => null });
    const base = baseFor(env, WIDGET_URLS.OlexSon30Gun());
    if (overview.status === 'failed') failed = true;
    else if (overview.status === 'blocked' && overview.state === 'module_off') writes.push(stateWrite(env, 'OlexSon30Gun', 'module_off'));
    else {
      const input = { overview: valueOf(overview), trend: valueOf(trend), forbidden: overview.status === 'blocked' };
      writes.push(
        timelineWrite('OlexSon30Gun', buildEntries('OlexSon30Gun', now, (at) => ({ ...base, ...withCard(env, 'OlexSon30Gun', buildKpi(input, dctx(at))) }), stale)),
      );
    }
  }

  // Bekleyenler
  if (rows.length === 0) writes.push(stateWrite(env, 'OlexBekleyenler', resolveState('OlexBekleyenler', audience)));
  else {
    const needsExpiring = rows.includes('expiring');
    const incomplete =
      (rows.includes('ready') && ready?.status !== 'ok') ||
      (rows.includes('claim') && claims?.status !== 'ok') ||
      (needsExpiring && overview.status !== 'ok');
    if (incomplete) failed = true;
    else {
      const expiring = valueOf(overview)?.series.find((s) => s.key === 'cards')?.points.find((p) => p.key === 'warranties_expiring')?.value ?? null;
      const input = { audience, ready: valueOf(ready), claims: valueOf(claims), expiring };
      const base = baseFor(env, WIDGET_URLS.OlexBekleyenler());
      writes.push(
        timelineWrite(
          'OlexBekleyenler',
          buildEntries('OlexBekleyenler', now, (at) => ({ ...base, ...withCard(env, 'OlexBekleyenler', buildPending(input, { t: env.t, locale: env.lang, now: at, brandLogo: () => null })) }), stale),
        ),
      );
    }
  }

  // Ciro: tutar varsayılan gizli (`revealed: false`).
  if (ciroState !== 'ok') writes.push(stateWrite(env, 'OlexCiro', ciroState));
  else if (pnl?.status === 'blocked') writes.push(stateWrite(env, 'OlexCiro', pnl.state));
  else if (pnl?.status === 'ok') {
    const base = baseFor(env, WIDGET_URLS.OlexCiro());
    const input = pnl.value;
    writes.push(
      timelineWrite('OlexCiro', buildEntries('OlexCiro', now, (at) => ({ ...base, ...buildRevenue(input, { t: env.t, locale: env.lang, now: at, brandLogo: () => null }) }), stale)),
    );
  } else failed = true;

  return { writes, failed };
}

/** Müşteri ya da bayi olmayan hesap (depo, merkez yönetimi): sekiz widget da rol dışı. */
function planNone(env: Env): Plan {
  return { writes: [...CUSTOMER_WIDGETS, ...DEALER_WIDGETS].map((name) => stateWrite(env, name, 'wrong_role')), failed: false };
}

async function run(me: MobileMe, ticket: SyncTicket, now: Date): Promise<boolean> {
  const lang = i18n.language;
  if (ticket.ownerChanged) {
    await wipeWidgets();
    if (!syncGate.canWrite(ticket)) return false;
  }

  const probe: AudienceInput = {
    kinds: kindsOf(me),
    permissions: new Set(me.permissions),
    superAdmin: Boolean(me.user.is_super_admin),
    enabledModules: null,
  };
  const kind = audienceOf(probe);
  const audience: AudienceInput = kind === 'dealer' ? { ...probe, enabledModules: await fetchEnabledModules() } : probe;

  let images: Images | null = null;
  try {
    images = await import('./images');
  } catch {
    images = null;
  }
  const marks = images ? await images.storeMarks() : { markUri: null, markMonoUri: null };

  const env: Env = {
    now,
    lang,
    rtl: isRTL(lang),
    t: translator(lang, 'widgets'),
    tc: translator(lang, 'customer'),
    images,
    marks: { markUri: marks.markUri ?? '', markMonoUri: marks.markMonoUri ?? '' },
    audience,
    keep: new Set<string>(),
  };
  const plan = kind === 'customer' ? await planCustomer(env) : kind === 'dealer' ? await planDealer(env) : planNone(env);

  // Çıkış bu sırada olduysa görseller de silinir; yeni bir eşitleme geçtiyse onun dosyalarına dokunulmaz.
  const aborted = () => {
    if (syncGate.generation() !== ticket.generation) images?.purgeAll();
    return false;
  };
  let failed = plan.failed;
  for (const w of plan.writes) {
    if (!syncGate.canWrite(ticket)) return aborted();
    const widget = widgetFor(w.name);
    if (!widget) continue;
    try {
      if (w.options !== undefined) widget.setConfigurationParameterEnum?.('vehicle', w.options ?? undefined);
      w.commit(widget);
    } catch {
      failed = true;
    }
  }
  if (!syncGate.canWrite(ticket)) return aborted();
  // Başarısız bir widget eski görsellerini kullanmaya devam eder: yalnız tam başarıda temizlenir.
  if (!failed) images?.reconcile(env.keep);
  return !failed;
}

/**
 * Widget'ları tazeler. Zorlanmayan çağrı 15 dakika kısıtına uyar; hata sessizdir.
 * `me` yoksa (çıkış) ya da bu derlemede widget yoksa hiçbir şey yapmaz.
 */
export async function syncWidgets(me: MobileMe | null, { force }: { force: boolean }): Promise<void> {
  if (!me || !widgetsAvailable()) return;
  const startedAt = Date.now();
  if (!syncGate.shouldRun(startedAt, force)) return;
  const ticket = syncGate.begin(`${me.user.uuid}:${me.active_organization_uuid ?? ''}`);
  try {
    if (await run(me, ticket, new Date(startedAt)) && syncGate.finish(ticket, startedAt)) saveSyncRecord();
  } catch {
    // Sessiz: widget eski içeriğini korur.
  } finally {
    syncGate.abort(ticket);
  }
}
