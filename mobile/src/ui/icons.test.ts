import assert from 'node:assert/strict';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { describe, it } from 'node:test';

// SF Symbols 4.2 ships with iOS 16.4, the app's deployment target (Expo modules' podspecs).
const TARGET: [number, number] = [4, 2];

/** First SF Symbols release of every symbol, from sf-symbols-typescript (expo-symbols' name types). */
function releases() {
  const require = createRequire(import.meta.url);
  const dts = fs.readFileSync(path.join(path.dirname(require.resolve('sf-symbols-typescript')), 'index.d.ts'), 'utf8');
  const out = new Map<string, [number, number]>();
  let cur: [number, number] | null = null;
  for (const line of dts.split('\n')) {
    const head = /^export type SFSymbols(\d+)_(\d+) =/.exec(line);
    if (head) {
      cur = [Number(head[1]), Number(head[2])];
      continue;
    }
    const sym = /^\s*\| '([^']+)'/.exec(line);
    if (sym && cur && !out.has(sym[1])) out.set(sym[1], cur);
  }
  return out;
}

describe('icons', () => {
  it('marks symbols newer than the deployment target with minIOS', () => {
    const src = fs.readFileSync(path.join(import.meta.dirname, 'icons.ts'), 'utf8');
    const rel = releases();
    const entries = [...src.matchAll(/(\w+): \{ sf: '([^']+)', lucide: \w+(, minIOS: (\d+))? \}/g)];
    assert.ok(entries.length > 50, 'icon entries parsed');
    for (const [, name, sf, , min] of entries) {
      const v = rel.get(sf);
      assert.ok(v, `${name}: unknown SF Symbol ${sf}`);
      const newer = v[0] > TARGET[0] || (v[0] === TARGET[0] && v[1] > TARGET[1]);
      if (newer) assert.ok(min, `${name}: ${sf} needs SF Symbols ${v.join('.')}, add minIOS`);
    }
  });
});
