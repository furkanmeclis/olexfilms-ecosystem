import assert from 'node:assert/strict';
import { afterEach, describe, it } from 'node:test';

import { setDigitStyle } from '../../core/digits.ts';
import { count, maskedMoney, money, plateText, staleAgo, timeLeft, timeLeftShort, updatedAt, waitDays } from './format.ts';

/** Anahtar ve parametreleri okunur metne çevirir: `key|a=1,b=2`. */
const t = (key: string, opts?: Record<string, unknown>) =>
  opts ? `${key}|${Object.entries(opts).map(([k, v]) => `${k}=${v}`).join(',')}` : key;

afterEach(() => setDigitStyle('arab'));

describe('timeLeft', () => {
  it('0 gün ve negatif süre bitti sayılır', () => {
    assert.equal(timeLeft(0, t), 'left_ended');
    assert.equal(timeLeft(-3, t), 'left_ended');
  });

  it('1 gün gün olarak kalır', () => {
    assert.equal(timeLeft(1, t), 'left_days|count=1');
    assert.equal(timeLeft(29, t), 'left_days|count=29');
  });

  it('11 ay yalnız ay gösterir', () => {
    assert.equal(timeLeft(335, t), 'left_months|count=11');
  });

  it('tam yıl yalnız yıl gösterir', () => {
    assert.equal(timeLeft(365 * 2 + 1, t), 'left_years|count=2');
  });

  it('yıl ve ay birlikte', () => {
    assert.equal(timeLeft(Math.round(365.25 * 2 + 30.4375 * 4 + 1), t), 'left_years_months|years=2,months=4');
  });
});

describe('timeLeftShort', () => {
  it('kilit dairesi için kısa anahtarları kullanır', () => {
    assert.equal(timeLeftShort(Math.round(365.25 * 2 + 30.4375 * 4 + 1), t), 'koruma.left_compact_years_months|years=2,months=4');
    assert.equal(timeLeftShort(365 * 2 + 1, t), 'koruma.left_compact_years|count=2');
    assert.equal(timeLeftShort(335, t), 'koruma.left_compact_months|count=11');
    assert.equal(timeLeftShort(1, t), 'koruma.left_compact_days|count=1');
  });

  it('bitmiş garanti 0 gün gösterir', () => {
    assert.equal(timeLeftShort(0, t), 'koruma.left_compact_days|count=0');
    assert.equal(timeLeftShort(-5, t), 'koruma.left_compact_days|count=0');
  });
});

describe('count', () => {
  it('tasarımdaki 1.248 gibi gruplar', () => {
    assert.equal(count(1248, 'tr'), '1.248');
    assert.equal(count(1248, 'en'), '1,248');
    assert.equal(count(7, 'tr'), '7');
  });

  it('Arapçada Arap rakamı, latn tercihinde Latin', () => {
    setDigitStyle('arab');
    assert.match(count(1248, 'ar'), /^[٠-٩٬,.\s]+$/);
    assert.doesNotMatch(count(1248, 'ar'), /[0-9]/);
    setDigitStyle('latn');
    assert.match(count(1248, 'ar'), /^[0-9٬,.\s]+$/);
    assert.match(count(1248, 'ar'), /1/);
  });
});

describe('money / maskedMoney', () => {
  it('kuruşsuz yazar', () => {
    assert.equal(money('12500.50', 'TRY', 'tr'), '₺12.501');
    assert.equal(money('0.00', 'TRY', 'tr'), '₺0');
  });

  it('negatif tutarda eksi korunur', () => {
    assert.match(money('-1200.00', 'TRY', 'tr'), /^-₺1\.200$/);
  });

  it('boş veya geçersiz tutar sıfır olur', () => {
    assert.equal(money('', 'TRY', 'tr'), '₺0');
    assert.equal(money('abc', 'TRY', 'tr'), '₺0');
  });

  it('maskeli tutarda sembol var, rakam yok', () => {
    assert.equal(maskedMoney('TRY', 'tr'), '₺•••••');
    for (const locale of ['tr', 'en', 'ar', 'de']) {
      assert.doesNotMatch(maskedMoney('TRY', locale), /[0-9٠-٩]/, locale);
    }
    setDigitStyle('arab');
    assert.doesNotMatch(maskedMoney('EUR', 'ar'), /[0-9٠-٩]/);
  });

  it('bilinmeyen para birimi çökmez', () => {
    assert.doesNotThrow(() => money('10', 'XX', 'tr'));
    assert.doesNotThrow(() => maskedMoney('XX', 'tr'));
  });
});

describe('updatedAt', () => {
  it('saat ve dakikayı common.updated_at içine koyar', () => {
    assert.equal(updatedAt(new Date(2026, 10, 3, 14, 32), t, 'tr'), 'common.updated_at|time=14:32');
  });
});

describe('staleAgo', () => {
  it('dakika, saat ve gün', () => {
    assert.equal(staleAgo(30_000, t), 'common.minutes_count|count=1');
    assert.equal(staleAgo(59 * 60_000, t), 'common.minutes_count|count=59');
    assert.equal(staleAgo(2 * 3_600_000, t), 'common.hours_count|count=2');
    assert.equal(staleAgo(23 * 3_600_000 + 59 * 60_000, t), 'common.hours_count|count=23');
    assert.equal(staleAgo(24 * 3_600_000, t), 'common.days_count|count=1');
    assert.equal(staleAgo(3 * 24 * 3_600_000, t), 'common.days_count|count=3');
  });

  it('negatif süre bir dakika sayılır', () => {
    assert.equal(staleAgo(-5, t), 'common.minutes_count|count=1');
  });
});

describe('waitDays', () => {
  it('yerel gün farkı, saat değil', () => {
    const now = new Date(2026, 10, 3, 0, 5);
    assert.equal(waitDays(new Date(2026, 10, 2, 23, 55), now), 1);
    assert.equal(waitDays(new Date(2026, 10, 3, 0, 1), now), 0);
    assert.equal(waitDays(new Date(2026, 9, 30, 12, 0), now), 4);
  });

  it('ISO metni kabul eder, gelecek ve geçersiz tarih 0', () => {
    const now = new Date(2026, 10, 3, 12, 0);
    assert.equal(waitDays(new Date(2026, 10, 1, 9, 0).toISOString(), now), 2);
    assert.equal(waitDays(new Date(2026, 10, 9, 9, 0), now), 0);
    assert.equal(waitDays('geçersiz', now), 0);
  });
});

describe('plateText', () => {
  it('plaka Arapçada da Latin ve değişmeden kalır', () => {
    setDigitStyle('arab');
    assert.equal(plateText('34 ABC 123'), '34 ABC 123');
  });

  it('yazılmış Arap rakamı Latin olur', () => {
    assert.equal(plateText('٣٤ ABC ١٢٣'), '34 ABC 123');
  });
});
