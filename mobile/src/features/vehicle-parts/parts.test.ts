import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  applyPreset,
  canonicalPartKey,
  isPresetActive,
  mixHex,
  schemeColors,
  selectablePartKeys,
  splitPartKeys,
  VEHICLE_PART_PRESETS,
  VEHICLE_PARTS,
} from './parts.ts';

const preset = (id: string) => VEHICLE_PART_PRESETS.find((p) => p.id === id)!;

describe('vehicle parts', () => {
  it('has 20 unique backend keys', () => {
    assert.equal(VEHICLE_PARTS.length, 20);
    assert.equal(new Set(VEHICLE_PARTS.map((p) => p.key)).size, 20);
  });

  it('maps legacy ids to backend keys and never throws on unknown ones', () => {
    assert.equal(canonicalPartKey('body_hood'), 'body_kaput');
    assert.equal(canonicalPartKey(' window_on_cam '), 'window_on_cam');
    assert.equal(canonicalPartKey('spoiler'), null);
    const r = splitPartKeys(['window_on_cam', 'body_hood', 'body_kaput', 'spoiler', '', 7 as unknown as string]);
    assert.deepEqual(r.known, ['body_kaput', 'window_on_cam']);
    assert.deepEqual(r.unknown, ['spoiler']);
    assert.deepEqual(splitPartKeys(null), { known: [], unknown: [] });
  });

  it('applies presets per group and toggles them off', () => {
    const all = selectablePartKeys();
    const front = applyPreset(new Set(['window_on_cam']), preset('body_front_three'), all);
    assert.deepEqual([...front].sort(), ['body_kaput', 'body_sag_on_camurluk', 'body_sol_on_camurluk', 'window_on_cam']);
    assert.ok(isPresetActive(front, preset('body_front_three'), all));
    assert.deepEqual([...applyPreset(front, preset('body_front_three'), all)], ['window_on_cam']);
  });

  it('limits presets to allowed keys', () => {
    const allowed = selectablePartKeys(['body_kaput', 'body_hood', 'nope']);
    assert.deepEqual([...allowed], ['body_kaput']);
    assert.deepEqual([...applyPreset(new Set(), preset('body_full'), allowed)], ['body_kaput']);
  });
});

describe('schemeColors', () => {
  const tokens = { muted: '#182a20', border: '#283e32', mutedForeground: '#8d9d92', primary: '#61c28e', success: '#00d492', card: '#12251b' };

  it('mixes the brand into a dark card and keeps it on the outline', () => {
    const c = schemeColors(tokens);
    assert.equal(c.bodyOn, mixHex(tokens.primary, tokens.card, 0.45));
    assert.equal(c.strokeOn, tokens.primary);
    assert.notEqual(c.bodyOn, tokens.primary);
  });

  it('keeps solid tokens on a light card', () => {
    const c = schemeColors({ ...tokens, primary: '#003d25', success: '#009966', card: '#fcfefc' });
    assert.equal(c.bodyOn, '#003d25');
    assert.equal(c.windowOn, '#009966');
  });
});

describe('mixHex', () => {
  it('blends two colors and leaves an unreadable one alone', () => {
    assert.equal(mixHex('#ffffff', '#000000', 0.5), '#808080');
    assert.equal(mixHex('oklch(0.5 0.1 150)', '#000000', 0.5), 'oklch(0.5 0.1 150)');
  });
});
