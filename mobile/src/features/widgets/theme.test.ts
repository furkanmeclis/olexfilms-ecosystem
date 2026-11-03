import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { DARK_PALETTE, LIGHT_PALETTE, WIDGET_THEME } from './theme.ts';

const HEX = /^#[0-9a-f]{6}$/;

describe('widget teması', () => {
  it('iki palet aynı anahtarlara sahip', () => {
    assert.deepEqual(Object.keys(LIGHT_PALETTE).sort(), Object.keys(DARK_PALETTE).sort());
  });

  it('tüm değerler #rrggbb', () => {
    for (const [mode, palette] of Object.entries(WIDGET_THEME)) {
      for (const [key, value] of Object.entries(palette)) {
        assert.match(value, HEX, `${mode}.${key}`);
      }
    }
  });

  it('iki temada aynı olan jetonlar eşit', () => {
    for (const key of ['onGreen', 'onGreenMuted', 'amber', 'plate', 'plateFg', 'plateStrip'] as const) {
      assert.equal(LIGHT_PALETTE[key], DARK_PALETTE[key], key);
    }
  });

  it('tasarım değerlerini korur', () => {
    assert.equal(LIGHT_PALETTE.pri, '#003d25');
    assert.equal(DARK_PALETTE.pri, '#61c28e');
    assert.equal(DARK_PALETTE.green, '#0b3a25');
    assert.equal(LIGHT_PALETTE.track, '#d7e1da');
  });
});
