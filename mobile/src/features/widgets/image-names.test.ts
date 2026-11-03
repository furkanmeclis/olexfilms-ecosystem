import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { MARK_FILE, MARK_MONO_FILE } from './config.ts';
import { heroFileName, isFresh, logoFileName, qrFileName, FRESH_MS, staleFiles } from './image-names.ts';

describe('dosya adları', () => {
  it('kapak adı model, marka, varsayılan sırasını izler', () => {
    assert.equal(heroFileName({ car_model: { uuid: 'abc-1' }, car_brand: { uuid: 'zzz' } }), 'hero-m-abc-1.png');
    assert.equal(heroFileName({ car_model: null, car_brand: { uuid: 'zzz' } }), 'hero-b-zzz.png');
    assert.equal(heroFileName({ car_model: null, car_brand: null }), 'hero-default.png');
  });

  it('logo ve QR adı', () => {
    assert.equal(logoFileName('b7f2-01'), 'logo-b7f2-01.png');
    assert.equal(qrFileName('AB12-cd_34'), 'qr-AB12-cd_34.png');
  });

  it('yol ayracı, nokta ve boşluk reddedilir', () => {
    for (const bad of ['../x', 'a/b', 'a\\b', 'a.b', '', 'a b', '..', 'a\0b']) {
      assert.equal(logoFileName(bad), null, bad);
      assert.equal(qrFileName(bad), null, bad);
    }
    assert.equal(heroFileName({ car_model: { uuid: '../etc' }, car_brand: null }), null);
  });

  it('çok uzun kod reddedilir', () => {
    assert.equal(qrFileName('a'.repeat(65)), null);
    assert.equal(qrFileName('a'.repeat(64)), `qr-${'a'.repeat(64)}.png`);
  });
});

describe('staleFiles', () => {
  it('listede olmayanları verir, işaretleri korur', () => {
    const existing = ['hero-m-1.png', 'hero-m-2.png', 'logo-b.png', MARK_FILE, MARK_MONO_FILE];
    assert.deepEqual(staleFiles(existing, new Set(['hero-m-1.png', 'logo-b.png'])), ['hero-m-2.png']);
  });

  it('boş liste işaretler dışındaki her şeyi verir', () => {
    assert.deepEqual(staleFiles(['a.png', MARK_FILE, 'qr-x.png'], new Set()), ['a.png', 'qr-x.png']);
  });

  it('fark yoksa boş', () => {
    assert.deepEqual(staleFiles(['a.png'], new Set(['a.png', 'b.png'])), []);
  });
});

describe('isFresh', () => {
  const now = Date.UTC(2026, 10, 3, 12);
  it('yedi günden yeni taze', () => {
    assert.equal(isFresh(now - FRESH_MS + 1, now), true);
    assert.equal(isFresh(now - 1000, now), true);
  });
  it('tam yedi gün ve eski bayat', () => {
    assert.equal(isFresh(now - FRESH_MS, now), false);
    assert.equal(isFresh(now - FRESH_MS - 1, now), false);
  });
  it('bilinmeyen zaman bayat, gelecek tarih bayat', () => {
    assert.equal(isFresh(null, now), false);
    assert.equal(isFresh(now + 60_000, now), false);
  });
  it('yedi gün 7*24 saat', () => {
    assert.equal(FRESH_MS, 7 * 24 * 3_600_000);
  });
});
