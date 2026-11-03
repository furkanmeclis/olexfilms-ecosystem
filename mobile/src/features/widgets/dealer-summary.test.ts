import assert from 'node:assert/strict';
import { after, before, describe, it } from 'node:test';

import type { components } from '@/core/api/schema';
import type { AudienceInput } from './audience.ts';
import {
  buildKpi,
  buildPanom,
  buildPending,
  buildQuick,
  buildRevenue,
  type DealerCtx,
  type ServiceFields,
  localDayStartIso,
  MAX_BAR,
  monthRange,
} from './dealer-summary.ts';

type Service = components['schemas']['Service'];
type Report = components['schemas']['ReportEnvelope'];

// Yerel gün hesapları Türkiye saatiyle sınanır; Date çağrıları test anında okur.
const originalTz = process.env.TZ;
before(() => {
  process.env.TZ = 'Europe/Istanbul';
});
after(() => {
  if (originalTz === undefined) delete process.env.TZ;
  else process.env.TZ = originalTz;
});

const t = (key: string, opts?: Record<string, unknown>) =>
  opts ? `${key}|${Object.entries(opts).map(([k, v]) => `${k}=${v}`).join(',')}` : key;

const NOW = new Date('2026-11-03T12:00:00Z');
const ctx: DealerCtx = {
  t,
  locale: 'tr',
  now: NOW,
  brandLogo: (id) => (id ? `file:///brand/${id}.png` : null),
};

const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;

// Uydurma kişi, telefon, not ve şasi: hiçbiri çıktıya girmemeli.
const SECRETS = ['Zeynep Örnektepe', '+905550001122', 'gizli-teslim-notu', 'WVWZZZ1JZXW000001', '98765.43', 'Gizli Bayi Ltd'];

function svc(n: number, status: Service['status'], createdAt: string, extra: Partial<Service> = {}): Service {
  return {
    uuid: id(n),
    service_no: `DS0000000${n}`,
    status,
    status_label: status,
    organization: { uuid: id(900), name: 'Gizli Bayi Ltd' },
    customer: { uuid: id(901), name: 'Zeynep Örnektepe', phone: '+905550001122' },
    vehicle_uuid: id(800 + n),
    car_brand: { uuid: id(600 + n), name: 'Marka' },
    car_model: { uuid: id(700 + n), name: `Model ${n}` },
    model_year: 2023,
    plate: '34 TST 001',
    plate_country: 'TR',
    vin: 'WVWZZZ1JZXW000001',
    km: 1200,
    package: null,
    notes: 'gizli-teslim-notu',
    income_amount: '98765.43',
    item_categories: ['Boya koruma', 'Cam filmi'],
    created_at: createdAt,
    completed_at: null,
    ...extra,
  } as unknown as Service;
}

const page = (items: Service[], total = items.length) => ({ items: items as ServiceFields[], total });

describe('buildPanom', () => {
  const next = page([
    svc(3, 'pending', '2026-11-02T10:00:00Z'),
    svc(1, 'draft', '2026-10-30T10:00:00Z'),
    svc(2, 'pending', '2026-11-01T10:00:00Z'),
    svc(4, 'pending', '2026-11-02T11:00:00Z'),
    svc(5, 'pending', '2026-11-02T12:00:00Z'),
  ], 12);
  const active = page([svc(6, 'processing', '2026-10-31T10:00:00Z')], 5);
  const done = page([svc(7, 'completed', '2026-10-01T10:00:00Z', { completed_at: '2026-11-03T05:00:00Z' })], 2);

  it('toplamlar sunucu total değerinden, satırlar en eski dört', () => {
    const out = buildPanom({ next, active, done, canCreate: true }, ctx);
    assert.equal(out.state, 'ok');
    assert.equal(out.next, '12');
    assert.equal(out.active, '5');
    assert.equal(out.done, '2');
    assert.equal(out.rows.length, 4);
    assert.deepEqual(out.rows.map((r) => r.uuid), [id(1), id(2), id(3), id(4)]);
  });

  it('satır alanları ve meta: ilk kategori + bekleme', () => {
    const [row] = buildPanom({ next, active, done, canCreate: true }, ctx).rows;
    assert.equal(row.url, `olexfilms://bayi/services/${id(1)}`);
    assert.equal(row.carTitle, 'Marka Model 1');
    assert.equal(row.plate, '34 TST 001');
    assert.equal(row.plateCountry, 'TR');
    assert.match(row.meta, /^Boya koruma · common\.days_count\|count=4$/);
    assert.equal(row.statusTone, 'neutral');
    assert.equal(row.logoUri, `file:///brand/${id(601)}.png`);
    assert.equal(row.initial, 'M');
  });

  it('longest en eski sıradaki hizmet', () => {
    const out = buildPanom({ next, active, done, canCreate: true }, ctx);
    assert.equal(out.longest?.uuid, id(1));
    assert.match(out.longest?.waitLabel ?? '', /^panom\.longest_wait\|wait=/);
  });

  it('bilinmeyen ve iptal durumları satıra girmez', () => {
    const odd = page([svc(1, 'cancelled', '2026-10-01T10:00:00Z'), svc(2, 'bogus' as Service['status'], '2026-10-02T10:00:00Z'), svc(3, 'pending', '2026-10-03T10:00:00Z')], 3);
    const out = buildPanom({ next: odd, active, done, canCreate: true }, ctx);
    assert.deepEqual(out.rows.map((r) => r.uuid), [id(3)]);
    assert.equal(out.longest?.uuid, id(3));
  });

  it('hepsi sıfırsa empty; yeni hizmet düğmesi yalnız yazma yetkisiyle', () => {
    const zero = { next: page([]), active: page([]), done: page([]) };
    const a = buildPanom({ ...zero, canCreate: true }, ctx);
    assert.equal(a.state, 'empty');
    assert.equal(a.card?.title, 'state.empty_queue_title');
    assert.equal(a.card?.cta, 'state.new_service');
    assert.equal(buildPanom({ ...zero, canCreate: false }, ctx).card?.cta, undefined);
    assert.equal(a.longest, null);
  });

  it('sıra boş ama işlemde varsa empty değil', () => {
    const out = buildPanom({ next: page([]), active, done: page([]), canCreate: true }, ctx);
    assert.equal(out.state, 'ok');
    assert.deepEqual(out.rows, []);
  });
});

describe('gizlilik: çıktıda müşteri, telefon, not, VIN, tutar yok', () => {
  const next = page([svc(1, 'pending', '2026-11-01T10:00:00Z')]);
  const ready = page([svc(2, 'ready', '2026-10-28T10:00:00Z')]);
  const audience: AudienceInput = { kinds: ['bayi'], permissions: new Set(['services.read', 'services.write', 'customers.read', 'warranty_claims.read', 'warranties.read']), superAdmin: false, enabledModules: null };

  it('Panom, Bekleyenler ve Hızlı işlem', () => {
    const outputs = [
      buildPanom({ next, active: ready, done: page([]), canCreate: true }, ctx),
      buildPending({ audience, ready, claims: { total: 2 }, expiring: 4 }, ctx),
      buildQuick(audience, ctx, 1284),
    ];
    for (const out of outputs) {
      const json = JSON.stringify(out);
      for (const secret of SECRETS) assert.ok(!json.includes(secret), `${secret} sızdı: ${json}`);
    }
  });
});

describe('buildQuick', () => {
  const base: AudienceInput = { kinds: ['bayi'], permissions: new Set(), superAdmin: false, enabledModules: null };

  it('yetkisiz yalnız Tara', () => {
    const out = buildQuick(base, ctx);
    assert.deepEqual(out.actions.map((a) => a.id), ['scan']);
    assert.equal(out.actions[0].url, 'olexfilms://bayi/scan');
    assert.equal(out.actions[0].label, 'quick.scan');
    assert.equal(out.customersCount, null);
  });

  it('müşteri sayısı yalnız müşteriler düğmesiyle', () => {
    const withCustomers = { ...base, permissions: new Set(['customers.read', 'services.write']) };
    const out = buildQuick(withCustomers, ctx, 1284);
    assert.deepEqual(out.actions.map((a) => a.id), ['scan', 'new_service', 'customers']);
    assert.equal(out.customersCount, 'quick.customers_count|count=1284');
    assert.equal(buildQuick(withCustomers, ctx).customersCount, null);
    const servicesOnly = { ...base, permissions: new Set(['services.read']) };
    assert.equal(buildQuick(servicesOnly, ctx, 99).customersCount, null);
    assert.equal(buildQuick(servicesOnly, ctx).actions[1].url, 'olexfilms://bayi/services');
  });
});

function card(key: string, value: number) {
  return { key, label: `sunucu etiketi ${key}`, value };
}
function overview(cards: { key: string; label: string; value: number }[]): Report {
  return { series: [{ key: 'cards', label: 'Kartlar', unit: '', points: cards }] } as unknown as Report;
}
function trend(values: number[]): Report {
  return {
    series: [{ key: 'created', label: 'Açılan', unit: '', points: values.map((value, i) => ({ key: `2026-10-${i + 1}`, label: `d${i}`, value })) }],
  } as unknown as Report;
}
const allCards = overview([
  card('services_created', 42),
  card('services_completed', 38),
  card('services_open', 9),
  card('warranties_active', 1248),
  card('warranties_expiring', 7),
  card('customers_total', 5),
]);

describe('buildKpi', () => {
  it('beş kart, bilinen sırada, çevrilmiş etiketle; bilinmeyen kart yok', () => {
    const out = buildKpi({ overview: allCards, trend: trend([1, 2, 3]) }, ctx);
    assert.equal(out.state, 'ok');
    assert.deepEqual(out.cards.map((c) => c.key), ['services_created', 'services_completed', 'services_open', 'warranties_active', 'warranties_expiring']);
    assert.deepEqual(out.cards.map((c) => c.label), ['kpi.created', 'kpi.completed', 'kpi.open', 'kpi.active_warranty', 'kpi.expiring']);
    assert.equal(out.cards[3].value, '1.248');
    assert.equal(out.primary?.value, '38');
    assert.equal(out.primary?.label, 'kpi.completed_service');
    assert.equal(out.summary, 'kpi.summary|created=42,open=9');
  });

  it('eksik kart gösterilmez; completed yoksa ilk mevcut kart küçükte', () => {
    const out = buildKpi({ overview: overview([card('services_open', 9), card('warranties_active', 3)]), trend: null }, ctx);
    assert.deepEqual(out.cards.map((c) => c.key), ['services_open', 'warranties_active']);
    assert.equal(out.primary?.key, 'services_open');
    assert.equal(out.primary?.label, 'kpi.open');
    assert.equal(out.summary, null);
    assert.deepEqual(out.bars, []);
  });

  it('kart yok ya da 403: no_permission', () => {
    assert.equal(buildKpi({ overview: overview([card('customers_total', 1)]), trend: null }, ctx).state, 'no_permission');
    const forbidden = buildKpi({ overview: null, trend: null, forbidden: true }, ctx);
    assert.equal(forbidden.state, 'no_permission');
    assert.deepEqual(forbidden.cards, []);
    assert.equal(forbidden.primary, null);
  });

  it('çubuklar: son beş nokta, en büyük MAX_BAR, son nokta current', () => {
    const out = buildKpi({ overview: allCards, trend: trend([100, 5, 10, 20, 40, 80, 40]) }, ctx);
    assert.equal(out.bars.length, 5);
    assert.deepEqual(out.bars.map((b) => b.height), [Math.round((10 / 80) * MAX_BAR), Math.round((20 / 80) * MAX_BAR), Math.round((40 / 80) * MAX_BAR), MAX_BAR, Math.round((40 / 80) * MAX_BAR)]);
    assert.deepEqual(out.bars.map((b) => b.current), [false, false, false, false, true]);
    assert.equal(MAX_BAR, 86);
    assert.equal(out.bars[4].label, 'kpi.this_week');
    assert.equal(out.bars[3].label, 'kpi.weeks_ago|count=1');
    assert.equal(out.bars[0].label, 'kpi.weeks_ago|count=4');
  });

  it('sıfır seri: yükseklikler 0, bölme hatası yok', () => {
    const out = buildKpi({ overview: allCards, trend: trend([0, 0, 0, 0, 0]) }, ctx);
    assert.deepEqual(out.bars.map((b) => b.height), [0, 0, 0, 0, 0]);
    const short = buildKpi({ overview: allCards, trend: trend([3, 6]) }, ctx);
    assert.equal(short.bars.length, 2);
    assert.equal(short.bars[1].height, MAX_BAR);
  });
});

describe('buildPending', () => {
  const all: AudienceInput = {
    kinds: ['bayi'],
    permissions: new Set(['services.read', 'warranty_claims.read', 'warranties.read']),
    superAdmin: false,
    enabledModules: null,
  };
  const ready = page([svc(2, 'ready', '2026-10-31T10:00:00Z'), svc(3, 'ready', '2026-10-29T10:00:00Z')], 6);

  it('üç satır, sayılar ve en eski hazır araç', () => {
    const out = buildPending({ audience: all, ready, claims: { total: 3 }, expiring: 11 }, ctx);
    assert.equal(out.state, 'ok');
    assert.deepEqual(out.rows.map((r) => [r.id, r.count]), [['ready', '6'], ['claim', '3'], ['expiring', '11']]);
    assert.equal(out.rows[0].detail, 'pending.ready_detail|vehicle=Marka Model 3,count=5');
    assert.equal(out.rows[0].url, 'olexfilms://bayi/services');
    assert.equal(out.rows[1].url, 'olexfilms://bayi/menu/warranty_claims');
    assert.equal(out.rows[2].url, 'olexfilms://bayi/menu/warranties');
    assert.equal(out.rows[1].detail, 'pending.claim_detail');
    assert.equal(out.rows[2].detail, 'pending.expiring_detail');
  });

  it('yetkisiz satırlar yok; hiç satır kalmazsa neden durumda', () => {
    const noClaims = { ...all, permissions: new Set(['services.read']) };
    assert.deepEqual(buildPending({ audience: noClaims, ready, claims: null, expiring: null }, ctx).rows.map((r) => r.id), ['ready']);
    const none = { ...all, permissions: new Set<string>() };
    assert.equal(buildPending({ audience: none, ready: null, claims: null, expiring: null }, ctx).state, 'no_permission');
    const off = { ...none, enabledModules: new Set<string>() };
    assert.equal(buildPending({ audience: off, ready: null, claims: null, expiring: null }, ctx).state, 'module_off');
  });

  it('hepsi sıfırsa empty', () => {
    const out = buildPending({ audience: all, ready: page([]), claims: { total: 0 }, expiring: 0 }, ctx);
    assert.equal(out.state, 'empty');
    assert.equal(out.rows[0].detail, '');
  });
});

describe('monthRange ve gün başı', () => {
  it('ay sınırları UTC', () => {
    assert.deepEqual(monthRange(new Date('2026-11-03T12:00:00Z')), { from: '2026-11-01', to: '2026-11-30' });
    assert.deepEqual(monthRange(new Date('2028-02-10T00:00:00Z')), { from: '2028-02-01', to: '2028-02-29' });
    assert.deepEqual(monthRange(new Date('2026-12-31T23:59:59Z')), { from: '2026-12-01', to: '2026-12-31' });
  });

  it('ay UTC takvimine göre: Türkiye yerel Kasım 1 02:00, UTC Ekim 31', () => {
    assert.deepEqual(monthRange(new Date('2026-10-31T23:00:00Z')), { from: '2026-10-01', to: '2026-10-31' });
  });

  it('yerel gün başı UTC+3: 00:00-03:00 arası da aynı yerel gün', () => {
    assert.equal(localDayStartIso(new Date('2026-11-02T22:30:00Z')), '2026-11-02T21:00:00.000Z');
    assert.equal(localDayStartIso(new Date('2026-11-03T12:00:00Z')), '2026-11-02T21:00:00.000Z');
    assert.equal(localDayStartIso(new Date('2026-11-03T20:59:59Z')), '2026-11-02T21:00:00.000Z');
    assert.equal(localDayStartIso(new Date('2026-11-03T21:00:00Z')), '2026-11-03T21:00:00.000Z');
  });
});

describe('buildRevenue', () => {
  const input = { currency: 'TRY', totals: { income: '125000.50', expense: '40000', net: '85000.50' } };

  it('varsayılan gizli; etiketler para biçimli', () => {
    const out = buildRevenue(input, ctx);
    assert.equal(out.state, 'ok');
    assert.equal(out.revealed, false);
    assert.equal(out.currency, 'TRY');
    assert.match(out.incomeLabel, /125\.001/);
    assert.match(out.netLabel, /85\.001/);
    assert.match(out.expenseLabel, /40\.000/);
    assert.match(out.maskedLabel, /•••••/);
    assert.ok(!/\d/.test(out.maskedLabel));
    assert.equal(out.monthTitle, 'revenue.month_income|month=Kasım');
    assert.equal(out.hiddenTitle, 'revenue.hidden');
    assert.equal(out.tapToReveal, 'revenue.tap_to_reveal');
  });
});
