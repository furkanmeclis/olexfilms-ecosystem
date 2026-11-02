import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { filterOptions, fitsInline, foldText, matchesQuery, needsSearch, pickerRows } from './search.ts';

const opts = [
  { key: 'a', title: 'İstanbul Kadıköy Bayi', subtitle: 'Kadıköy, İstanbul' },
  { key: 'b', title: 'Ankara Çankaya', keywords: 'ANK-01' },
  { key: 'c', title: 'Şişli Film Center' },
  { key: 'd', title: 'مركز الرياض' },
];

describe('foldText', () => {
  it('drops case and marks, Turkish i forms included', () => {
    assert.equal(foldText('İSTANBUL'), 'istanbul');
    assert.equal(foldText('ıspARTA'), 'isparta');
    assert.equal(foldText('Şişli  Çankaya'), 'sisli cankaya');
  });
  it('drops Arabic harakat', () => {
    assert.equal(foldText('مَرْكَز'), 'مركز');
  });
});

describe('matchesQuery', () => {
  it('needs every word somewhere in the fields', () => {
    assert.equal(matchesQuery(['Kadıköy Bayi', 'İstanbul'], 'istanbul kadikoy'), true);
    assert.equal(matchesQuery(['Kadıköy Bayi'], 'kadikoy ankara'), false);
  });
  it('matches anything for an empty query and skips empty fields', () => {
    assert.equal(matchesQuery([null, undefined], '  '), true);
    assert.equal(matchesQuery([null, 'x'], 'x'), true);
  });
});

describe('filterOptions', () => {
  it('keeps everything for a blank query, as a copy', () => {
    const out = filterOptions(opts, ' ');
    assert.deepEqual(out, opts);
    assert.notEqual(out, opts);
  });
  it('searches title, subtitle and keywords, keeping order', () => {
    assert.deepEqual(filterOptions(opts, 'istanbul').map((o) => o.key), ['a']);
    assert.deepEqual(filterOptions(opts, 'ank-01').map((o) => o.key), ['b']);
    assert.deepEqual(filterOptions(opts, 'sisli').map((o) => o.key), ['c']);
    assert.deepEqual(filterOptions(opts, 'الرياض').map((o) => o.key), ['d']);
    assert.deepEqual(filterOptions(opts, 'zzz'), []);
  });
});

describe('needsSearch / fitsInline', () => {
  it('shows search above seven local options or for server lists', () => {
    assert.equal(needsSearch(7, false), false);
    assert.equal(needsSearch(8, false), true);
    assert.equal(needsSearch(0, true), true);
  });
  it('keeps up to five data-driven options inline', () => {
    assert.equal(fitsInline(5), true);
    assert.equal(fitsInline(6), false);
  });
});

describe('pickerRows', () => {
  it('adds a header before each run of a section', () => {
    const rows = pickerRows([
      { key: '1', title: 'A', section: 'Past' },
      { key: '2', title: 'B', section: 'Past' },
      { key: '3', title: 'C', section: 'All' },
    ]);
    assert.deepEqual(
      rows.map((r) => (r.kind === 'header' ? `#${r.title}` : r.option.key)),
      ['#Past', '1', '2', '#All', '3'],
    );
  });
  it('adds no header without sections and keeps keys unique', () => {
    const rows = pickerRows([
      { key: '1', title: 'A' },
      { key: '2', title: 'B' },
    ]);
    assert.equal(rows.length, 2);
    assert.equal(new Set(rows.map((r) => r.key)).size, 2);
  });
});
