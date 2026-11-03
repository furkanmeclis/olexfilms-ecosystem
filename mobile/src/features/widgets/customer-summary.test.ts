import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import type { components } from '@/core/api/schema';
import { MAX_VEHICLES } from './config.ts';
import { buildHizmet, buildKart, buildKoruma, type CustomerCtx } from './customer-summary.ts';

type Detail = components['schemas']['PortalVehicleDetail'];
type Warranty = components['schemas']['PortalActiveWarranty'];
type ServiceItem = components['schemas']['PortalServiceListItem'];

/** Anahtar ve parametreleri okunur metne çevirir: `key|a=1,b=2`. */
const t = (key: string, opts?: Record<string, unknown>) =>
  opts ? `${key}|${Object.entries(opts).map(([k, v]) => `${k}=${v}`).join(',')}` : key;
const tc = (key: string, opts?: Record<string, unknown>) => `c:${t(key, opts)}`;

const ctx: CustomerCtx = {
  t,
  tc,
  locale: 'tr',
  assets: {
    hero: (id) => `file:///hero/${id}.jpg`,
    brandLogo: (id) => (id ? `file:///brand/${id}.png` : null),
    qr: (code) => `file:///qr/${code}.png`,
  },
};

const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;

function warranty(n: number, daysLeft: number, percentLeft: number, name = `Film ${n}`): Warranty {
  return {
    uuid: id(100 + n),
    public_code: `W${n}`,
    start_at: '2025-01-01T00:00:00Z',
    end_at: '2027-01-01T00:00:00Z',
    days_left: daysLeft,
    percent_left: percentLeft,
    product: { uuid: id(200 + n), sku: `SKU${n}`, name },
    service: { uuid: id(300 + n), service_no: `DS${n}` },
    organization: { uuid: id(400), name: 'Deneme Bayi', type: 'dealer' },
  };
}

function service(n: number, status: string, createdAt: string, extra: Partial<ServiceItem> = {}): ServiceItem {
  return {
    uuid: id(500 + n),
    service_no: `DS0000000${n}`,
    status: status as ServiceItem['status'],
    package: null,
    organization: { uuid: id(400), name: 'Deneme Bayi', type: 'dealer' },
    vehicle_uuid: id(1),
    car_brand_name: 'Marka',
    car_model_name: 'Model',
    model_year: 2022,
    plate: '34 ABC 123',
    plate_country: 'TR',
    completed_at: status === 'completed' ? createdAt : null,
    created_at: createdAt,
    ...extra,
  };
}

function vehicle(n: number, warranties: Warranty[], services: ServiceItem[] = [], extra: Partial<Detail> = {}): Detail {
  return {
    uuid: id(n),
    car_brand: { uuid: id(600 + n), name: 'Marka' },
    car_model: { uuid: id(700 + n), name: `Model ${n}` },
    model_year: 2022,
    plate: '34 ABC 123',
    plate_country: 'TR',
    vin: null,
    service_count: services.length,
    active_warranty_count: warranties.length,
    last_service_at: '2026-03-04T10:00:00Z',
    created_at: '2025-01-01T00:00:00Z',
    service_summary: { total: services.length, completed: 0, organization_count: 1, last_service_at: '2026-03-04T10:00:00Z' },
    services,
    active_warranties: warranties,
    ...extra,
  };
}

describe('buildKoruma sıralama', () => {
  it('en kısa süreli garanti taşıyan araç önce, garantisiz sonda', () => {
    const out = buildKoruma(
      [vehicle(1, []), vehicle(2, [warranty(2, 400, 60)]), vehicle(3, [warranty(3, 90, 20), warranty(4, 900, 90)])],
      ctx,
    );
    assert.deepEqual(out.vehicles.map((v) => v.id), [id(3), id(2), id(1)]);
    assert.equal(out.state, 'ok');
  });

  it('eşit sürede uuid eşitliği bozar', () => {
    const out = buildKoruma([vehicle(9, [warranty(9, 100, 50)]), vehicle(2, [warranty(2, 100, 50)])], ctx);
    assert.deepEqual(out.vehicles.map((v) => v.id), [id(2), id(9)]);
  });

  it('MAX_VEHICLES ile keser', () => {
    const many = Array.from({ length: MAX_VEHICLES + 4 }, (_, i) => vehicle(i + 1, [warranty(i + 1, 100 + i, 50)]));
    const out = buildKoruma(many, ctx);
    assert.equal(out.vehicles.length, MAX_VEHICLES);
    assert.equal(out.vehicles[0].id, id(1));
  });
});

describe('buildKoruma araç içeriği', () => {
  const v = vehicle(1, [warranty(1, 800, 80, 'Uzun'), warranty(2, 60, 14, 'Kısa'), warranty(3, 200, 15, 'Orta'), warranty(4, 300, 40, 'Dördüncü')]);

  it('garantiler artan days_left, en çok üç', () => {
    const [out] = buildKoruma([v], ctx).vehicles;
    assert.deepEqual(out.warranties.map((w) => w.name), ['Kısa', 'Orta', 'Dördüncü']);
    assert.equal(out.activeCountLabel, 'koruma.active_count|count=4');
  });

  it('eşik: percent_left 14 amber, 15 değil', () => {
    const [out] = buildKoruma([v], ctx).vehicles;
    assert.equal(out.warranties[0].warn, true);
    assert.equal(out.warranties[0].percent, 14);
    assert.equal(out.warranties[1].warn, false);
    assert.equal(out.warranties[1].percent, 15);
  });

  it('süre etiketleri iki namespace ile, görseller parametreden', () => {
    const [out] = buildKoruma([v], ctx).vehicles;
    assert.match(out.warranties[0].leftLabel, /^c:left_/);
    assert.match(out.warranties[0].leftShortLabel, /^koruma\.left_compact_/);
    assert.match(out.warranties[0].leftLongLabel, /^koruma\.left_long\|time=c:left_/);
    assert.equal(out.heroUri, `file:///hero/${id(1)}.jpg`);
    assert.equal(out.brandLogoUri, `file:///brand/${id(601)}.png`);
    assert.equal(out.brandInitial, 'M');
    assert.equal(out.url, `olexfilms://musteri/vehicle/${id(1)}`);
    assert.match(out.lastServiceLabel, /^koruma\.last_service\|date=/);
  });

  it('uzun ad kesilmez, plaka Latin rakamlı', () => {
    const long = 'Çok Uzun Bir Marka Adı Modeli Gerçekten Çok Uzun Bir Ad';
    const [out] = buildKoruma([vehicle(1, [warranty(1, 100, 50)], [], { car_brand: { uuid: id(1), name: long }, plate: '٣٤ ABC ١٢٣' })], ctx).vehicles;
    assert.ok(out.title.startsWith(long));
    assert.equal(out.plate, '34 ABC 123');
  });
});

describe('buildKoruma boş haller', () => {
  it('araç yok', () => {
    const out = buildKoruma([], ctx);
    assert.equal(out.state, 'empty');
    assert.equal(out.card?.title, 'state.no_vehicle_title');
    assert.equal(out.card?.body, 'state.no_vehicle_body');
    assert.deepEqual(out.vehicles, []);
  });

  it('garanti yok: eyebrow araç adı', () => {
    const out = buildKoruma([vehicle(1, [])], ctx);
    assert.equal(out.state, 'ok');
    assert.equal(out.vehicles[0].emptyCard?.eyebrow, 'Marka Model 1');
    assert.equal(out.vehicles[0].emptyCard?.title, 'state.no_warranty_title');
  });

  it('listeden silinmiş ya da süresi bitmiş garanti sayılmaz', () => {
    const [out] = buildKoruma([vehicle(1, [warranty(1, 0, 0), warranty(2, -3, 0)])], ctx).vehicles;
    assert.deepEqual(out.warranties, []);
    assert.equal(out.activeCountLabel, 'koruma.active_count|count=0');
    assert.ok(out.emptyCard);
  });

  it('marka adı yoksa genel araç adı', () => {
    const [out] = buildKoruma([vehicle(1, [], [], { car_brand: null, car_model: null })], ctx).vehicles;
    assert.equal(out.title, 'common.auto_vehicle');
    assert.equal(out.brandLogoUri, null);
  });
});

describe('buildKart', () => {
  it('her araç için en kısa garanti kartı; garantisiz araç yok', () => {
    const out = buildKart([vehicle(1, []), vehicle(2, [warranty(5, 500, 50), warranty(6, 100, 40)])], ctx);
    assert.equal(out.state, 'ok');
    assert.equal(out.vehicles.length, 1);
    const [k] = out.vehicles;
    assert.equal(k.publicCode, 'W6');
    assert.equal(k.productName, 'Film 6');
    assert.match(k.untilLabel, /^card\.until\|date=/);
    assert.equal(k.qrUri, 'file:///qr/W6.png');
    assert.equal(k.heroUri, `file:///hero/${id(2)}.jpg`);
    assert.equal(k.title, 'Marka Model 2');
    assert.equal(k.plate, '34 ABC 123');
  });

  it('araç yok ve garanti yok boş halleri', () => {
    assert.equal(buildKart([], ctx).card?.title, 'state.no_vehicle_title');
    const none = buildKart([vehicle(1, [])], ctx);
    assert.equal(none.state, 'empty');
    assert.equal(none.card?.title, 'state.no_warranty_title');
  });
});

describe('buildHizmet', () => {
  const T0 = '2026-05-01T10:00:00Z';
  const T1 = '2026-05-02T10:00:00Z';
  const T2 = '2026-05-03T10:00:00Z';
  const pick = (services: ServiceItem[]) => buildHizmet([{ vehicle: vehicle(1, [], services) }], ctx).vehicles[0];

  it('açık hizmet varsa en yenisi; adımlar done/current/todo', () => {
    const v = pick([service(1, 'processing', T0), service(2, 'pending', T1), service(3, 'completed', T2)]);
    assert.equal(v.service?.uuid, id(502));
    assert.deepEqual(v.service?.steps.map((s) => s.phase), ['current', 'todo', 'todo', 'todo']);
    assert.equal(v.service?.statusLabel, 'service.steps.pending');
    assert.equal(v.service?.done, false);
    assert.equal(v.service?.url, `olexfilms://musteri/service/${id(502)}`);
    assert.equal(v.service?.dealerName, 'Deneme Bayi');
    assert.equal(v.service?.serviceNo, 'DS00000002');
  });

  it('ready: ilk iki adım tamam, üçüncü şu an', () => {
    const v = pick([service(1, 'ready', T0)]);
    assert.deepEqual(v.service?.steps.map((s) => s.phase), ['done', 'done', 'current', 'todo']);
    assert.deepEqual(v.service?.steps.map((s) => s.label), [
      'service.steps.pending',
      'service.steps.processing',
      'service.steps.ready',
      'service.steps.completed',
    ]);
  });

  it('draft müşteride pending gösterilir', () => {
    const v = pick([service(1, 'draft', T0)]);
    assert.equal(v.service?.steps[0].phase, 'current');
    assert.equal(v.service?.statusLabel, 'service.steps.pending');
  });

  it('açık yoksa en yeni completed: tüm adımlar done', () => {
    const v = pick([service(1, 'completed', T0), service(2, 'completed', T1)]);
    assert.equal(v.service?.uuid, id(502));
    assert.equal(v.service?.done, true);
    assert.deepEqual(v.service?.steps.map((s) => s.phase), ['done', 'done', 'done', 'done']);
    assert.equal(v.service?.statusLabel, 'service.steps.completed');
  });

  it('son kayıt cancelled ise boş durum', () => {
    const v = pick([service(1, 'completed', T0), service(2, 'cancelled', T1)]);
    assert.equal(v.service, null);
    assert.equal(v.emptyCard?.title, 'service.no_open_title');
    assert.equal(v.emptyCard?.eyebrow, 'Marka Model 1');
  });

  it('hizmet yok: boş durum', () => {
    const v = pick([]);
    assert.equal(v.service, null);
    assert.equal(v.emptyCard?.body, 'service.no_open_body');
  });

  it('PortalService de kabul edilir (dealer.name)', () => {
    const detail = {
      uuid: id(900),
      service_no: 'DS99999999',
      status: 'processing',
      status_label: 'x',
      vehicle_uuid: id(1),
      car_brand: { uuid: id(1), name: 'M' },
      car_model: { uuid: id(2), name: 'N' },
      model_year: null,
      plate: null,
      created_at: T0,
      completed_at: null,
      applied_parts: [],
      products: [],
      dealer: { uuid: id(3), name: 'Detay Bayi', city: '', district: '', address: '', whatsapp: null },
      warranties: [],
    } as components['schemas']['PortalService'];
    const out = buildHizmet([{ vehicle: vehicle(1, []), services: [detail] }], ctx).vehicles[0];
    assert.equal(out.service?.dealerName, 'Detay Bayi');
    assert.equal(out.service?.steps[1].phase, 'current');
  });

  it('araç yok boş hali; sıra ve kesme Koruma ile aynı', () => {
    assert.equal(buildHizmet([], ctx).state, 'empty');
    const out = buildHizmet(
      [{ vehicle: vehicle(1, []) }, { vehicle: vehicle(2, [warranty(2, 50, 50)]) }],
      ctx,
    );
    assert.deepEqual(out.vehicles.map((v) => v.id), [id(2), id(1)]);
  });
});
