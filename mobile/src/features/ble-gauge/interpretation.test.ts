import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { SAPL_COLORS, SAPL_DEFAULT_REFERENCE, saplLevel } from './interpretation.ts';

describe('saplLevel (Interpretation, §7)', () => {
  it('uses the general outside limits', () => {
    const cases: [number, number][] = [
      [0, 0], [69, 0], [70, 1], [165, 1], [166, 2], [300, 2], [301, 3], [500, 3], [501, 4], [700, 4], [701, 5],
    ];
    for (const [um, level] of cases) assert.equal(saplLevel(um), level, `${um} µm`);
  });

  it('uses the general inside limits', () => {
    const cases: [number, number][] = [[39, 0], [40, 1], [90, 1], [91, 2], [150, 2], [151, 3], [350, 3], [351, 4], [550, 4], [551, 5]];
    for (const [um, level] of cases) assert.equal(saplLevel(um, 'inside'), level, `${um} µm`);
  });

  it('uses limits around the reference in individual mode', () => {
    const r = SAPL_DEFAULT_REFERENCE.outside;
    assert.equal(saplLevel(r - 36, 'outside', r), 0);
    assert.equal(saplLevel(r - 35, 'outside', r), 1);
    assert.equal(saplLevel(r + 35, 'outside', r), 1);
    assert.equal(saplLevel(r + 170, 'outside', r), 2);
    assert.equal(saplLevel(r + 571, 'outside', r), 5);
    const ri = SAPL_DEFAULT_REFERENCE.inside;
    assert.equal(saplLevel(ri + 30, 'inside', ri), 1);
    assert.equal(saplLevel(ri + 31, 'inside', ri), 2);
  });

  it('has the app colors', () => {
    assert.equal(SAPL_COLORS[1], '#55D37A');
    assert.equal(SAPL_COLORS[4], SAPL_COLORS[5]);
  });
});
