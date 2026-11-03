import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { SYNC_MIN_INTERVAL_MS } from './config.ts';
import { createSyncGate } from './sync-gate.ts';

const T0 = 1_800_000_000_000;

describe('kısıt', () => {
  it('ilk çalıştırma serbest', () => {
    assert.equal(createSyncGate().shouldRun(T0, false), true);
  });

  it('15 dakikadan önce reddeder, sonra izin verir', () => {
    const gate = createSyncGate();
    gate.finish(gate.begin('u:o'), T0);
    assert.equal(gate.shouldRun(T0 + SYNC_MIN_INTERVAL_MS - 1, false), false);
    assert.equal(gate.shouldRun(T0 + SYNC_MIN_INTERVAL_MS, false), true);
  });

  it('zorla kısıtı aşar', () => {
    const gate = createSyncGate();
    gate.finish(gate.begin('u:o'), T0);
    assert.equal(gate.shouldRun(T0 + 1000, true), true);
  });

  it('kayıtlı zaman diskten gelir', () => {
    const gate = createSyncGate();
    gate.restore({ generatedAt: T0, owner: 'u:o' });
    assert.equal(gate.shouldRun(T0 + 60_000, false), false);
  });

  it('saat geriye gittiyse kısıt takılı kalmaz', () => {
    const gate = createSyncGate();
    gate.restore({ generatedAt: T0, owner: 'u:o' });
    assert.equal(gate.shouldRun(T0 - 60_000, false), true);
  });

  it('süren eşitleme varken zorlanmayan çağrı reddedilir', () => {
    const gate = createSyncGate();
    gate.begin('u:o');
    assert.equal(gate.shouldRun(T0, false), false);
    assert.equal(gate.shouldRun(T0, true), true);
  });

  it('başarısız (kaydedilmeyen) çalıştırma kısıt başlatmaz', () => {
    const gate = createSyncGate();
    const ticket = gate.begin('u:o');
    gate.abort(ticket);
    assert.equal(gate.shouldRun(T0 + 1000, false), true);
    assert.equal(gate.snapshot().generatedAt, null);
  });
});

describe('sıra dışı', () => {
  it('yalnız en son başlayan yazabilir', () => {
    const gate = createSyncGate();
    const older = gate.begin('u:o');
    const newer = gate.begin('u:o');
    assert.equal(gate.canWrite(older), false);
    assert.equal(gate.canWrite(newer), true);
  });

  it('eski çalıştırmanın bitişi zamanı ve çalışma durumunu bozmaz', () => {
    const gate = createSyncGate();
    const older = gate.begin('u:o');
    const newer = gate.begin('u:o');
    assert.equal(gate.finish(older, T0), false);
    assert.equal(gate.snapshot().generatedAt, null);
    assert.equal(gate.shouldRun(T0, false), false);
    assert.equal(gate.finish(newer, T0 + 5), true);
    assert.equal(gate.snapshot().generatedAt, T0 + 5);
  });
});

describe('çıkış', () => {
  it('invalidate sonrası uçuştaki yanıt yazılamaz', () => {
    const gate = createSyncGate();
    const ticket = gate.begin('u:o');
    assert.equal(gate.canWrite(ticket), true);
    gate.invalidate();
    assert.equal(gate.canWrite(ticket), false);
    assert.equal(gate.finish(ticket, T0), false);
    assert.deepEqual(gate.snapshot(), { generatedAt: null, owner: null });
  });

  it('invalidate kısıtı ve sahibi sıfırlar', () => {
    const gate = createSyncGate();
    gate.finish(gate.begin('u:o'), T0);
    gate.invalidate();
    assert.equal(gate.shouldRun(T0 + 1000, false), true);
    assert.equal(gate.begin('u:o').ownerChanged, false);
  });

  it('invalidate nesli artırır', () => {
    const gate = createSyncGate();
    const before = gate.generation();
    gate.invalidate();
    assert.equal(gate.generation(), before + 1);
  });
});

describe('sahip değişimi', () => {
  it('kayıtlı sahip yoksa temizlik gerekmez', () => {
    assert.equal(createSyncGate().begin('u1:o1').ownerChanged, false);
  });

  it('aynı sahip değişim sayılmaz', () => {
    const gate = createSyncGate();
    gate.restore({ generatedAt: T0, owner: 'u1:o1' });
    assert.equal(gate.begin('u1:o1').ownerChanged, false);
  });

  it('kullanıcı ya da kuruluş değişince değişim sayılır', () => {
    const a = createSyncGate();
    a.restore({ generatedAt: T0, owner: 'u1:o1' });
    assert.equal(a.begin('u2:o1').ownerChanged, true);
    const b = createSyncGate();
    b.restore({ generatedAt: T0, owner: 'u1:o1' });
    assert.equal(b.begin('u1:o2').ownerChanged, true);
  });

  it('değişim bir kez bildirilir', () => {
    const gate = createSyncGate();
    gate.restore({ generatedAt: T0, owner: 'u1:o1' });
    assert.equal(gate.begin('u2:o1').ownerChanged, true);
    assert.equal(gate.begin('u2:o1').ownerChanged, false);
  });

  it('kaydedilen sahip son başarılı çalıştırmanınkidir', () => {
    const gate = createSyncGate();
    gate.finish(gate.begin('u2:o1'), T0);
    assert.deepEqual(gate.snapshot(), { generatedAt: T0, owner: 'u2:o1' });
  });
});
