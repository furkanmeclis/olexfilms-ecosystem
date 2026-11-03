import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { planPending } from './pending-plan.ts';

describe('Bekleyenler satır kararı', () => {
  it('hepsi ok ise tüm satırlar yazılır', () => {
    assert.deepEqual(planPending(['ready', 'claim', 'expiring'], { ready: 'ok', claim: 'ok', expiring: 'ok' }), { kind: 'write', dropped: [] });
  });

  it('engelli özet bitecek garanti satırını düşürür, diğerleri kalır', () => {
    assert.deepEqual(planPending(['ready', 'expiring'], { ready: 'ok', expiring: 'blocked' }), { kind: 'write', dropped: ['expiring'] });
  });

  it('engelli teslime hazır satırı düşer, yazım atlanmaz', () => {
    assert.deepEqual(planPending(['ready', 'claim'], { ready: 'blocked', claim: 'ok' }), { kind: 'write', dropped: ['ready'] });
  });

  it('engelli talep satırı düşer, yazım atlanmaz', () => {
    assert.deepEqual(planPending(['ready', 'claim'], { ready: 'ok', claim: 'blocked' }), { kind: 'write', dropped: ['claim'] });
  });

  it('engel yüzünden satır kalmadıysa no_permission', () => {
    assert.deepEqual(planPending(['claim'], { claim: 'blocked' }), { kind: 'blocked' });
    assert.deepEqual(planPending(['expiring'], { expiring: 'blocked' }), { kind: 'blocked' });
    assert.deepEqual(planPending(['ready', 'claim', 'expiring'], { ready: 'blocked', claim: 'blocked', expiring: 'blocked' }), { kind: 'blocked' });
  });

  it('gerçek hata yazımı atlar; engel varsa bile', () => {
    assert.deepEqual(planPending(['ready', 'claim'], { ready: 'failed', claim: 'ok' }), { kind: 'failed' });
    assert.deepEqual(planPending(['ready', 'claim'], { ready: 'blocked', claim: 'failed' }), { kind: 'failed' });
    assert.deepEqual(planPending(['expiring'], { expiring: 'failed' }), { kind: 'failed' });
  });

  it('istenmiş ama sonucu olmayan satır hata sayılır', () => {
    assert.deepEqual(planPending(['ready'], { ready: undefined }), { kind: 'failed' });
  });

  it('satır olmayanın durumuna bakılmaz', () => {
    assert.deepEqual(planPending(['ready'], { ready: 'ok', expiring: 'failed', claim: 'failed' }), { kind: 'write', dropped: [] });
  });
});
