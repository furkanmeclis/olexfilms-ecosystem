import assert from 'node:assert/strict';
import { test } from 'node:test';

import { buildTargets, guideView, nextOpen, nextPartIndex, partProgress, targetKey, type PointReading } from './guided.ts';
import type { PartMapData } from './point-map.ts';

const map: PartMapData = {
  point_radius: 22,
  assets: [
    { part: 'LEFT_SIDE', place: 'left', kind: 'main', points: [], svg: '<svg viewBox="0 0 2048 1024"><rect/></svg>' },
    {
      part: 'LEFT_FRONT_DOOR',
      place: 'left',
      kind: 'element',
      points: [
        { count: 2, x: 20, y: 20 },
        { count: 1, x: 10, y: 10 },
      ],
      svg: '<svg><g id="LEFT_FRONT_DOOR"><path fill="#FFFFFF"/></g><g id="LEFT_FRONT_DOOR_points"></g></svg>',
    },
    { part: 'TOP_SIDE', place: 'top', kind: 'main', points: [], svg: '<svg></svg>' },
    { part: 'ROOF', place: 'top', kind: 'element', points: [{ count: 1, x: 5, y: 5 }], svg: null },
    { part: 'HOOD', place: 'top', kind: 'element', points: [{ count: 1, x: 1, y: 1 }, { count: null, x: 0, y: 0 }], svg: null },
  ],
  // The backend's walk (svg.Guide): hood, left side, roof.
  guide: [
    { part: 'HOOD', place: 'top', position: 1, x: 1, y: 1 },
    { part: 'LEFT_FRONT_DOOR', place: 'left', position: 1, x: 10, y: 10 },
    { part: 'LEFT_FRONT_DOOR', place: 'left', position: 2, x: 20, y: 20 },
    { part: 'ROOF', place: 'top', position: 1, x: 5, y: 5 },
  ],
};

const reading = (um: number): PointReading => ({ um, level: 1, material: 'fe', at: '2026-11-11T00:00:00Z' });

test('targets are the backend guide, in its order', () => {
  const t = buildTargets(map);
  assert.deepEqual(
    t.map((x) => targetKey(x)),
    ['HOOD#1', 'LEFT_FRONT_DOOR#1', 'LEFT_FRONT_DOOR#2', 'ROOF#1'],
  );
  assert.equal(t[1]!.place, 'left');
});

test('nextOpen skips measured points and wraps; -1 when all are done', () => {
  const t = buildTargets(map);
  const r = { 'LEFT_FRONT_DOOR#1': reading(100) };
  assert.equal(nextOpen(t, r, 0), 2);
  assert.equal(nextOpen(t, r, 3), 0);
  const all = Object.fromEntries(t.map((x) => [targetKey(x), reading(100)]));
  assert.equal(nextOpen(t, all, 0), -1);
});

test('nextPartIndex jumps to the first point of the next part', () => {
  const t = buildTargets(map);
  assert.equal(nextPartIndex(t, 1), 3);
  assert.equal(nextPartIndex(t, 3), -1);
});

test('partProgress counts done and total per part', () => {
  const t = buildTargets(map);
  assert.deepEqual(partProgress(t, { 'LEFT_FRONT_DOOR#2': reading(90) }), [
    { part: 'HOOD', first: 0, done: 0, total: 1 },
    { part: 'LEFT_FRONT_DOOR', first: 1, done: 1, total: 2 },
    { part: 'ROOF', first: 3, done: 0, total: 1 },
  ]);
});

test('guideView highlights the active part, numbers its points and rings the current one', () => {
  const t = buildTargets(map);
  const svg = guideView(map, t[1]!, { 'LEFT_FRONT_DOOR#2': reading(90) }, {
    highlight: '#00FF00',
    ring: '#0000FF',
    levelColor: () => '#55D37A',
  })!;
  assert.match(svg, /fill="#00FF00"/);
  assert.match(svg, /stroke="#0000FF"/);
  assert.match(svg, /fill="#55D37A"/);
  assert.match(svg, />1<\/text>/);
  assert.match(svg, />2<\/text>/);
  assert.equal(guideView(map, { part: 'X', place: 'back', position: 1, x: 0, y: 0 }, {}, { highlight: '', ring: '', levelColor: () => '' }), null);
});
