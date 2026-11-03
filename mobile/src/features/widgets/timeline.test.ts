import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { PROPS_BUDGET_BYTES, STALE_AFTER_MS, WIDGET_NAMES } from './config.ts';
import { staleAgo } from './format.ts';
import type { TFn } from './format.ts';
import { WIDGET_THEME } from './theme.ts';
import { buildEntries, daysLeftAt, staleLabelFor } from './timeline.ts';
import type { KorumaProps, WidgetState } from './types.ts';

const HOUR = 3_600_000;
const DAY = 24 * HOUR;
const NOW = new Date(2026, 10, 3, 14, 30, 15);

type Props = { state: WidgetState; staleLabel: string | null; at: number; left: number };

const label = (ms: number) => `stale|${Math.round(ms / HOUR)}h`;
const make =
  (state: WidgetState = 'ok') =>
  (at: Date): Props => ({ state, staleLabel: null, at: at.getTime(), left: daysLeftAt(100, NOW, at) });

function entriesOf(name: (typeof WIDGET_NAMES)[number], state: WidgetState = 'ok') {
  const result = buildEntries(name, NOW, make(state), label);
  assert.equal(result.mode, 'timeline');
  return result.mode === 'timeline' ? result.entries : [];
}

describe('günlük widget\'lar', () => {
  for (const name of ['OlexKoruma', 'OlexGarantiKarti'] as const) {
    it(`${name}: şimdi + yedi yerel gece yarısı`, () => {
      const e = entriesOf(name);
      assert.equal(e.length, 8);
      assert.equal(e[0].date.getTime(), NOW.getTime());
      for (let k = 1; k <= 7; k++) assert.equal(e[k].date.getTime(), new Date(2026, 10, 3 + k).getTime());
    });

    it(`${name}: yalnız yedinci gün bayat`, () => {
      const e = entriesOf(name);
      assert.ok(e.slice(0, 7).every((x) => x.props.staleLabel === null));
      assert.equal(e[7].props.staleLabel, label(e[7].date.getTime() - NOW.getTime()));
    });
  }

  it('gece yarısı atlamasında days_left bir azalır', () => {
    const e = entriesOf('OlexKoruma');
    assert.deepEqual(e.map((x) => x.props.left), [100, 99, 98, 97, 96, 95, 94, 93]);
  });

  it('23:59 de ertesi gece yarısını atlar, aynı gün tekrarlanmaz', () => {
    const late = new Date(2026, 10, 3, 23, 59);
    const result = buildEntries('OlexKoruma', late, (at) => ({ state: 'ok' as const, staleLabel: null, left: daysLeftAt(10, late, at) }), label);
    assert.ok(result.mode === 'timeline');
    if (result.mode !== 'timeline') return;
    assert.equal(result.entries[1].date.getTime(), new Date(2026, 10, 4).getTime());
    assert.equal(result.entries[1].props.left, 9);
  });

  it('süresi biten garanti o girdide düşer', () => {
    const build = (at: Date) => {
      const left = daysLeftAt(2, NOW, at);
      return { state: 'ok' as const, staleLabel: null, warranties: left > 0 ? [`w${left}`] : [] };
    };
    const result = buildEntries('OlexGarantiKarti', NOW, build, label);
    assert.ok(result.mode === 'timeline');
    if (result.mode !== 'timeline') return;
    assert.deepEqual(result.entries.map((x) => x.props.warranties.length), [1, 1, 0, 0, 0, 0, 0, 0]);
  });
});

describe('diğer widget\'lar', () => {
  const others = WIDGET_NAMES.filter((n) => n !== 'OlexKoruma' && n !== 'OlexGarantiKarti');

  for (const name of others) {
    it(`${name}: şimdi, bayatlık, +6s, +24s; sıralı ve tekrarsız`, () => {
      const e = entriesOf(name);
      const offsets = e.map((x) => x.date.getTime() - NOW.getTime());
      const expected = [...new Set([0, STALE_AFTER_MS[name], 6 * HOUR, 24 * HOUR])].sort((a, b) => a - b);
      assert.deepEqual(offsets, expected);
      assert.ok(e.length <= 4);
    });

    it(`${name}: bayatlık sınırı ve sonrası dolu, öncesi boş`, () => {
      const e = entriesOf(name);
      for (const x of e) {
        const age = x.date.getTime() - NOW.getTime();
        assert.equal(x.props.staleLabel, age >= STALE_AFTER_MS[name] ? label(age) : null, `${age}`);
      }
    });
  }

  it('iki saatlik widget +6s ve +24s için bayat metni taşır', () => {
    const e = entriesOf('OlexPanom');
    assert.deepEqual(e.map((x) => x.props.staleLabel), [null, label(2 * HOUR), label(6 * HOUR), label(24 * HOUR)]);
  });
});

describe('ok olmayan durumlar', () => {
  for (const state of ['signed_out', 'wrong_role', 'no_permission', 'module_off', 'empty'] as const) {
    it(`${state}: tek anlık görüntü, bayat metin yok`, () => {
      for (const name of WIDGET_NAMES) {
        const result = buildEntries(name, NOW, make(state), label);
        assert.equal(result.mode, 'snapshot');
        if (result.mode === 'snapshot') {
          assert.equal(result.props.state, state);
          assert.equal(result.props.staleLabel, null);
          assert.equal(result.props.at, NOW.getTime());
        }
      }
    });
  }
});

describe('sınırlar', () => {
  it('hiçbir widget sekiz girdiyi aşmaz', () => {
    for (const name of WIDGET_NAMES) assert.ok(entriesOf(name).length <= 8, name);
  });

  it('build her girdi için kendi zamanıyla çağrılır', () => {
    const seen: number[] = [];
    buildEntries('OlexCiro', NOW, (at) => (seen.push(at.getTime()), { state: 'ok' as const, staleLabel: null }), label);
    assert.deepEqual(seen, [0, 6 * HOUR, 24 * HOUR].map((d) => NOW.getTime() + d));
  });

  it('build staleLabel doldurmuşsa ok girdide bayat olmayanda dokunulmaz', () => {
    const result = buildEntries('OlexCiro', NOW, () => ({ state: 'ok' as const, staleLabel: 'özel' }), label);
    assert.ok(result.mode === 'timeline');
    if (result.mode === 'timeline') assert.equal(result.entries[0].props.staleLabel, 'özel');
  });
});

describe('daysLeftAt', () => {
  it('takvim günü farkı; saatten bağımsız', () => {
    assert.equal(daysLeftAt(10, new Date(2026, 10, 3, 23, 59), new Date(2026, 10, 4, 0, 0)), 9);
    assert.equal(daysLeftAt(10, new Date(2026, 10, 3, 0, 1), new Date(2026, 10, 3, 23, 59)), 10);
    assert.equal(daysLeftAt(10, NOW, new Date(NOW.getTime() + 7 * DAY)), 3);
  });
});

describe('staleLabelFor', () => {
  const t: TFn = (key, options) => `${key}|${Object.entries(options ?? {}).map(([k, v]) => `${k}=${v}`).join(',')}`;
  it('state.stale_body içine staleAgo süresini koyar', () => {
    assert.equal(staleLabelFor(t)(2 * HOUR), `state.stale_body|time=${staleAgo(2 * HOUR, t)}`);
  });
});

describe('props bütçesi', () => {
  const long = 'Çok Uzun Bir Araç Adı Mercedes-Benz E 200 Exclusive Şanzıman Güvenlik Paketi';
  const vehicle = (i: number): KorumaProps['vehicles'][number] => ({
    id: `00000000-0000-4000-8000-00000000000${i}`,
    url: `olexfilms://garanti/arac/00000000-0000-4000-8000-00000000000${i}`,
    title: long,
    plate: '34 ÇĞŞ 1234',
    plateCountry: 'TR',
    heroUri: `file:///private/var/mobile/Containers/Shared/AppGroup/00000000-0000-4000-8000-000000000000/ExpoWidgets/hero-m-00000000-0000-4000-8000-00000000000${i}.png`,
    brandLogoUri: `file:///private/var/mobile/Containers/Shared/AppGroup/00000000-0000-4000-8000-000000000000/ExpoWidgets/logo-00000000-0000-4000-8000-00000000000${i}.png`,
    brandInitial: 'M',
    activeCountLabel: '3 aktif garanti ve uzun bir açıklama metni',
    lastServiceLabel: 'Son hizmet: 12 Ekim 2026, uzun bayi adı ile birlikte',
    warranties: [0, 1, 2].map((k) => ({
      name: `${long} ${k}`,
      leftLabel: '2 yıl 4 ay 12 gün',
      leftShortLabel: '2y 4a',
      percent: 80,
      warn: false,
    })),
    emptyCard: null,
  });
  const props = (at: Date): KorumaProps => ({
    state: 'ok',
    rtl: false,
    theme: WIDGET_THEME,
    markUri: 'file:///private/var/mobile/Containers/Shared/AppGroup/00000000-0000-4000-8000-000000000000/ExpoWidgets/olex-mark.png',
    markMonoUri: 'file:///private/var/mobile/Containers/Shared/AppGroup/00000000-0000-4000-8000-000000000000/ExpoWidgets/olex-mark-mono.png',
    url: 'olexfilms://garanti',
    updatedLabel: `Güncellendi ${at.getHours()}:00`,
    staleLabel: null,
    eyebrow: 'OLEX KORUMA',
    leftCaption: 'kalan',
    vehicles: Array.from({ length: 10 }, (_, i) => vehicle(i)),
  });

  it('on araç, en uzun metinlerle her girdi bütçe altında', () => {
    const result = buildEntries('OlexKoruma', NOW, props, staleLabelFor((k, o) => `${k} ${JSON.stringify(o)}`));
    assert.ok(result.mode === 'timeline');
    if (result.mode !== 'timeline') return;
    for (const e of result.entries) {
      const bytes = new TextEncoder().encode(JSON.stringify(e.props)).length;
      assert.ok(bytes <= PROPS_BUDGET_BYTES, `${bytes} > ${PROPS_BUDGET_BYTES}`);
    }
  });

  it('Türkçe uzun değerler korunur', () => {
    const result = buildEntries('OlexKoruma', NOW, props, label);
    assert.ok(result.mode === 'timeline');
    if (result.mode !== 'timeline') return;
    const last = result.entries.at(-1)?.props;
    assert.equal(last?.vehicles[0].title, long);
    assert.equal(last?.vehicles[9].plate, '34 ÇĞŞ 1234');
    assert.equal(last?.vehicles[9].warranties[2].name, `${long} 2`);
  });
});
