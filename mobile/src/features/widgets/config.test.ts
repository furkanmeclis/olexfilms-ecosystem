import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, it } from 'node:test';

import {
  CUSTOMER_WIDGETS,
  DEALER_WIDGETS,
  MAX_VEHICLES,
  PROPS_BUDGET_BYTES,
  STALE_AFTER_MS,
  SYNC_MIN_INTERVAL_MS,
  WIDGET_NAMES,
} from './config.ts';

const MOBILE_ROOT = fileURLToPath(new URL('../../../', import.meta.url));
const SRC_ROOT = join(MOBILE_ROOT, 'src');

type WidgetEntry = {
  name: string;
  android?: unknown;
  ios?: {
    supportedFamilies: string[];
    contentMarginsDisabled?: boolean;
    configuration?: { parameters: Record<string, unknown> };
  };
};

type PluginProps = { bundleIdentifier: string; groupIdentifier: string; widgets: WidgetEntry[] };

function readPlugin(): PluginProps {
  const app = JSON.parse(readFileSync(join(MOBILE_ROOT, 'app.json'), 'utf8')) as {
    expo: { plugins: unknown[] };
  };
  const entry = app.expo.plugins.find((p) => Array.isArray(p) && p[0] === 'expo-widgets') as
    | [string, PluginProps]
    | undefined;
  assert.ok(entry, 'app.json içinde expo-widgets eklentisi yok');
  return entry[1];
}

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) out.push(...sourceFiles(path));
    else out.push(path);
  }
  return out;
}

const SMALL = 'systemSmall';
const MEDIUM = 'systemMedium';
const LARGE = 'systemLarge';
const CIRCULAR = 'accessoryCircular';
const RECT = 'accessoryRectangular';
const INLINE = 'accessoryInline';
const ALL_FAMILIES = [SMALL, MEDIUM, LARGE, CIRCULAR, RECT, INLINE];

const EXPECTED: Record<string, { families: string[]; vehicle: boolean }> = {
  OlexKoruma: { families: ALL_FAMILIES, vehicle: true },
  OlexGarantiKarti: { families: [SMALL, MEDIUM], vehicle: true },
  OlexHizmetDurumu: { families: [SMALL, MEDIUM], vehicle: true },
  OlexPanom: { families: ALL_FAMILIES, vehicle: false },
  OlexHizliIslem: { families: [SMALL, MEDIUM, CIRCULAR], vehicle: false },
  OlexSon30Gun: { families: [SMALL, MEDIUM, LARGE], vehicle: false },
  OlexBekleyenler: { families: [SMALL, MEDIUM], vehicle: false },
  OlexCiro: { families: [SMALL, MEDIUM], vehicle: false },
};

describe('widget config', () => {
  const plugin = readPlugin();

  it('WIDGET_NAMES app.json ile aynı kümedir', () => {
    assert.deepEqual([...WIDGET_NAMES].sort(), plugin.widgets.map((w) => w.name).sort());
  });

  it('müşteri ve bayi listeleri tüm adları bölüştürür', () => {
    assert.deepEqual([...CUSTOMER_WIDGETS, ...DEALER_WIDGETS].sort(), [...WIDGET_NAMES].sort());
  });

  it('android tümünde null, iOS kenar boşlukları kapalı', () => {
    for (const w of plugin.widgets) {
      assert.equal(w.android, null, `${w.name}.android`);
      assert.equal(w.ios?.contentMarginsDisabled, true, `${w.name}.contentMarginsDisabled`);
    }
  });

  it('aileler ve araç seçimi tabloyla eşit', () => {
    for (const w of plugin.widgets) {
      const expected = EXPECTED[w.name];
      assert.ok(expected, `${w.name} tabloda yok`);
      assert.deepEqual(w.ios?.supportedFamilies, expected.families, `${w.name} aileler`);
      const params = w.ios?.configuration?.parameters;
      assert.equal(params !== undefined && 'vehicle' in params, expected.vehicle, `${w.name} vehicle`);
    }
  });

  it('araç parametresi: dinamik enum, varsayılan otomatik', () => {
    for (const w of plugin.widgets) {
      if (!EXPECTED[w.name].vehicle) continue;
      assert.deepEqual(
        w.ios?.configuration?.parameters.vehicle,
        { title: 'Araç', type: 'enum', dynamic: true, default: 'auto', values: [{ name: 'Otomatik', value: 'auto' }] },
        w.name,
      );
    }
  });

  it('kimlikler yalnız app.json içindedir', () => {
    assert.ok(plugin.bundleIdentifier);
    assert.ok(plugin.groupIdentifier);
    const needles = [plugin.bundleIdentifier, plugin.groupIdentifier];
    for (const file of sourceFiles(SRC_ROOT)) {
      if (!/\.(ts|tsx|js|jsx|json|mjs)$/.test(file)) continue;
      const text = readFileSync(file, 'utf8');
      for (const needle of needles) {
        assert.ok(!text.includes(needle), `${file} kimlik değerini içeriyor`);
      }
    }
  });

  it('bayatlık süreleri her widget için tanımlı', () => {
    assert.deepEqual(Object.keys(STALE_AFTER_MS).sort(), [...WIDGET_NAMES].sort());
    assert.equal(STALE_AFTER_MS.OlexKoruma, 7 * 24 * 60 * 60_000);
    assert.equal(STALE_AFTER_MS.OlexCiro, 24 * 60 * 60_000);
    assert.equal(STALE_AFTER_MS.OlexPanom, 2 * 60 * 60_000);
  });

  it('sayısal sınırlar', () => {
    assert.equal(MAX_VEHICLES, 10);
    assert.equal(SYNC_MIN_INTERVAL_MS, 900_000);
    assert.equal(PROPS_BUDGET_BYTES, 32_768);
  });
});
