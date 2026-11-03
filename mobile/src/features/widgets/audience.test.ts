import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { audienceOf, can, moduleOn, pendingBlockedState, pendingRows, quickActions, resolveState } from './audience.ts';
import type { AudienceInput } from './audience.ts';
import { CUSTOMER_WIDGETS, DEALER_WIDGETS, WIDGET_NAMES } from './config.ts';

function input(over: Partial<AudienceInput> = {}): AudienceInput {
  return { kinds: ['bayi'], permissions: new Set(), superAdmin: false, enabledModules: null, ...over };
}

const perms = (...p: string[]) => new Set(p);
const ALL_DEALER = perms(
  'services.read',
  'services.write',
  'measurements.read',
  'customers.read',
  'accounting.read',
  'warranty_claims.read',
  'warranties.read',
);

describe('audienceOf', () => {
  it('müşteri, bayi ve diğerleri', () => {
    assert.equal(audienceOf(input({ kinds: ['musteri'] })), 'customer');
    assert.equal(audienceOf(input({ kinds: ['bayi'] })), 'dealer');
    assert.equal(audienceOf(input({ kinds: ['admin'] })), 'none');
    assert.equal(audienceOf(input({ kinds: ['depo'] })), 'none');
    assert.equal(audienceOf(input({ kinds: [] })), 'none');
  });

  it('müşteri ve bayi birlikteyse müşteri kazanır', () => {
    assert.equal(audienceOf(input({ kinds: ['bayi', 'musteri'] })), 'customer');
  });

  it('bayi ile merkez birlikteyse bayi', () => {
    assert.equal(audienceOf(input({ kinds: ['admin', 'bayi'] })), 'dealer');
  });
});

describe('can / moduleOn', () => {
  it('süper admin her izne sahiptir', () => {
    assert.equal(can(input({ superAdmin: true }), 'accounting.read'), true);
    assert.equal(can(input(), 'accounting.read'), false);
    assert.equal(can(input({ permissions: perms('accounting.read') }), 'accounting.read'), true);
  });

  it('modül bilinmiyorsa açık', () => {
    assert.equal(moduleOn(input({ enabledModules: null }), 'accounting'), true);
    assert.equal(moduleOn(input({ enabledModules: new Set(['accounting']) }), 'accounting'), true);
    assert.equal(moduleOn(input({ enabledModules: new Set() }), 'accounting'), false);
  });

  it('süper admin kapalı modülü açmaz', () => {
    assert.equal(moduleOn(input({ superAdmin: true, enabledModules: new Set() }), 'accounting'), false);
  });
});

describe('resolveState: kitle', () => {
  it('müşteri üç müşteri widgetında ok, bayi widgetlarında wrong_role', () => {
    const customer = input({ kinds: ['musteri'] });
    for (const name of CUSTOMER_WIDGETS) assert.equal(resolveState(name, customer), 'ok', name);
    for (const name of DEALER_WIDGETS) assert.equal(resolveState(name, customer), 'wrong_role', name);
  });

  it('bayi müşteri widgetlarında wrong_role', () => {
    const dealer = input({ permissions: ALL_DEALER });
    for (const name of CUSTOMER_WIDGETS) assert.equal(resolveState(name, dealer), 'wrong_role', name);
    for (const name of DEALER_WIDGETS) assert.equal(resolveState(name, dealer), 'ok', name);
  });

  it('merkez işyeri, salt depo ve süper admin merkez sekizinde de wrong_role', () => {
    for (const kinds of [['admin'], ['depo'], ['admin', 'depo']] as const) {
      for (const superAdmin of [false, true]) {
        const i = input({ kinds: [...kinds], superAdmin, permissions: ALL_DEALER });
        for (const name of WIDGET_NAMES) assert.equal(resolveState(name, i), 'wrong_role', `${kinds}/${name}`);
      }
    }
  });
});

describe('resolveState: izin ve modül', () => {
  it('Panom services.read ister', () => {
    assert.equal(resolveState('OlexPanom', input()), 'no_permission');
    assert.equal(resolveState('OlexPanom', input({ permissions: perms('services.read') })), 'ok');
    assert.equal(resolveState('OlexPanom', input({ superAdmin: true })), 'ok');
  });

  it('Son 30 gün 403 dönerse no_permission', () => {
    assert.equal(resolveState('OlexSon30Gun', input()), 'ok');
    assert.equal(resolveState('OlexSon30Gun', input(), { forbidden: true }), 'no_permission');
  });

  it('Ciro accounting.read ve accounting modülü ister', () => {
    const base = { permissions: perms('accounting.read') };
    assert.equal(resolveState('OlexCiro', input()), 'no_permission');
    assert.equal(resolveState('OlexCiro', input(base)), 'ok');
    assert.equal(resolveState('OlexCiro', input({ ...base, enabledModules: new Set(['services']) })), 'module_off');
    assert.equal(resolveState('OlexCiro', input({ ...base, enabledModules: null })), 'ok');
  });

  it('Ciro: izin yok ve modül kapalıysa module_off', () => {
    assert.equal(resolveState('OlexCiro', input({ enabledModules: new Set() })), 'module_off');
  });

  it('Hızlı işlem hiç yetki istemez', () => {
    assert.equal(resolveState('OlexHizliIslem', input()), 'ok');
    assert.equal(resolveState('OlexHizliIslem', input({ enabledModules: new Set() })), 'ok');
  });

  it('Bekleyenler: erişilebilir satır varsa ok', () => {
    assert.equal(resolveState('OlexBekleyenler', input({ permissions: perms('warranties.read') })), 'ok');
  });

  it('Bekleyenler: hiç satır yoksa no_permission', () => {
    assert.equal(resolveState('OlexBekleyenler', input()), 'no_permission');
  });

  it('Bekleyenler: hiç satır yok ve talep modülü kapalıysa module_off', () => {
    assert.equal(resolveState('OlexBekleyenler', input({ enabledModules: new Set(['services']) })), 'module_off');
  });
});

describe('quickActions', () => {
  const ids = (i: AudienceInput) => quickActions(i).map((a) => a.id);

  it('yetkisiz bayide yalnız tara', () => {
    assert.deepEqual(ids(input()), ['scan']);
  });

  it('tam yetkide sıra tara, yeni hizmet, ölçüm, müşteriler', () => {
    assert.deepEqual(ids(input({ permissions: ALL_DEALER })), ['scan', 'new_service', 'measure', 'customers']);
  });

  it('süper admin dördü de alır, dörtten fazla olmaz', () => {
    const list = ids(input({ superAdmin: true }));
    assert.deepEqual(list, ['scan', 'new_service', 'measure', 'customers']);
    assert.ok(list.length <= 4);
  });

  it('customers.read yokken müşteriler yok, services.read varsa hizmetler gelir', () => {
    const list = ids(input({ permissions: perms('services.read', 'services.write', 'measurements.read') }));
    assert.deepEqual(list, ['scan', 'new_service', 'measure', 'services']);
    assert.ok(!list.includes('customers'));
  });

  it('ölçüm sekmesi services.write ister: yalnız measurements.read ile ölçüm düğmesi yok (gizli sekmeye gidemez)', () => {
    const list = ids(input({ permissions: perms('services.read', 'measurements.read') }));
    assert.ok(!list.includes('measure'));
  });

  it('customers.read da services.read da yoksa ikisi de yok', () => {
    const list = ids(input({ permissions: perms('services.write', 'measurements.read') }));
    assert.deepEqual(list, ['scan', 'new_service', 'measure']);
  });

  it('customers.read varken services.read hizmetler eklemez', () => {
    const list = ids(input({ permissions: perms('customers.read', 'services.read') }));
    assert.deepEqual(list, ['scan', 'customers']);
  });

  it('ölçüm modülü kapalıysa ölçüm yok', () => {
    const list = ids(input({ permissions: ALL_DEALER, enabledModules: new Set(['services']) }));
    assert.deepEqual(list, ['scan', 'new_service', 'customers']);
  });

  it('tara her zaman ilk, küçük boy buradan alır', () => {
    for (const permissions of [perms(), ALL_DEALER, perms('customers.read')]) {
      assert.equal(quickActions(input({ permissions }))[0]?.id, 'scan');
    }
  });
});

describe('pendingRows', () => {
  const rows = (i: AudienceInput) => pendingRows(i);

  it('üç satır, sırayla', () => {
    assert.deepEqual(rows(input({ permissions: ALL_DEALER })), ['ready', 'claim', 'expiring']);
  });

  it('erişilemeyen satır gizlenir', () => {
    assert.deepEqual(rows(input({ permissions: perms('services.read', 'warranties.read') })), ['ready', 'expiring']);
    assert.deepEqual(rows(input({ permissions: perms('warranty_claims.read') })), ['claim']);
  });

  it('talep modülü kapalıysa talep satırı gizlenir', () => {
    const i = input({ permissions: ALL_DEALER, enabledModules: new Set(['services']) });
    assert.deepEqual(rows(i), ['ready', 'expiring']);
  });

  it('enabledModules null iken talep satırı görünür', () => {
    assert.ok(rows(input({ permissions: perms('warranty_claims.read') })).includes('claim'));
  });

  it('pendingBlockedState satır varken ok', () => {
    assert.equal(pendingBlockedState(input({ permissions: ALL_DEALER })), 'ok');
  });

  it('pendingBlockedState modül kapalıysa module_off, değilse no_permission', () => {
    assert.equal(pendingBlockedState(input({ enabledModules: new Set() })), 'module_off');
    assert.equal(pendingBlockedState(input({ enabledModules: null })), 'no_permission');
  });
});
