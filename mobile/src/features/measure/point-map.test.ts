import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { averageLevel, composeView, partBody, partPoints, type MapValue, type PartMapData } from './point-map.ts';

const v = (
  part: string,
  position: number,
  um: string | null,
  interpretation: number | null,
  inside = false,
): MapValue => ({
  part_type: part,
  position,
  value_um: um,
  interpretation,
  is_inside: inside,
});

const hood =
  '<svg viewBox="0 0 10 10">\n  <g id="HOOD">\n<metadata>x</metadata><g fill="#FFFFFF"><path d="M0 0"/></g>\n  </g>\n  <g id="HOOD_points"><g id="HOOD_point_1"/></g>\n</svg>';
const map: PartMapData = {
  point_radius: 22,
  assets: [
    {
      part: 'TOP_SIDE',
      place: 'top',
      kind: 'main',
      points: [],
      svg: '<?xml version="1.0"?><svg viewBox="0 0 10 10"><path/></svg>',
    },
    {
      part: 'HOOD',
      place: 'top',
      kind: 'element',
      points: [
        { count: 1, x: 5, y: 6.5 },
        { count: 2, x: 7, y: 8 },
      ],
      svg: hood,
    },
    { part: 'ROOF', place: 'top', kind: 'element', points: [], svg: null },
  ],
};

describe('point map', () => {
  it('colors the part body without metadata and points', () => {
    const body = partBody(hood, 'HOOD', '#55d37a');
    assert.ok(body.includes('fill="#55d37a"'));
    assert.ok(!body.includes('metadata'));
    assert.ok(!body.includes('HOOD_points'));
  });

  it('composes the view with numbered points in their level color', () => {
    const svg = composeView(map, 'top', [v('HOOD', 1, '250.00', 3)])!;
    assert.ok(!svg.includes('<?xml'));
    assert.ok(svg.includes('<circle cx="5" cy="6.5" r="22" fill="#ec4a08"'));
    assert.ok(svg.includes('<circle cx="7" cy="8" r="22" fill="#9CA3AF"'));
    assert.ok(svg.includes('fill="#ec4a08"><path'));
    assert.equal(composeView(map, 'left', []), null);
  });

  it('averages the levels of a part', () => {
    assert.equal(averageLevel([v('HOOD', 1, '1', 1), v('HOOD', 2, '1', 2), v('HOOD', 3, null, -1)], 'HOOD'), 2);
    assert.equal(averageLevel([], 'HOOD'), null);
  });

  it('lists the µm of each point per part, outside and inside', () => {
    const out = partPoints(map, [
      v('ROOF', 2, '130.00', 1),
      v('HOOD', 1, '112.00', 1),
      v('HOOD', 1, '115.50', 2),
      v('HOOD', 2, '98.00', 1, true),
      v('BED', 1, null, null),
    ]);
    assert.deepEqual(
      out.map((p) => p.part),
      ['HOOD', 'ROOF', 'BED'],
    );
    assert.deepEqual(out[0].outside, [{ position: 1, um: 115.5, level: 2 }]);
    assert.deepEqual(out[0].inside, [{ position: 2, um: 98, level: 1 }]);
    assert.deepEqual(out[2].outside, [{ position: 1, um: null, level: -1 }]);
  });
});
