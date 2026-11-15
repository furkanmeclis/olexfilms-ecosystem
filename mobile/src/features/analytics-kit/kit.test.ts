import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { deltaPct, deltaTone, direction, formatDelta } from './delta.ts';
import { chartSeries, groupTimeSeries, hasPrevious, isTimeSeries, kpisOf, previousRange, rowLink, rowStatus, seriesTotal } from './envelope.ts';
import { bucketLabel, compact, formatValue, guessGranularity } from './format.ts';
import { CATEGORICAL, seriesColor, statusOfTone, withAlpha } from './palette.ts';
import {
  allowedGranularities,
  DEFAULT_PERIOD,
  effectiveGranularity,
  monthGrid,
  rangeDays,
  rangeError,
  tapRange,
  toQuery,
} from './period.ts';
import { arcPath, bandLayout, donutAngles, extent, labelIndices, linear, linePath, nearestIndex, niceStep, niceTicks, pointX } from './scale.ts';
import { type Column, aggregateOf, alignOf, fold, nextSort, searchRows, sortRows, totals } from './table.ts';

describe('palette', () => {
  it('keeps the fixed categorical order and never cycles', () => {
    assert.equal(CATEGORICAL.light.length, 8);
    assert.equal(CATEGORICAL.dark.length, 8);
    assert.equal(seriesColor(0, 'light'), '#2a78d6');
    assert.equal(seriesColor(1, 'dark'), '#d95926');
    assert.equal(seriesColor(7, 'light'), '#e34948');
    assert.equal(seriesColor(8, 'light'), null);
    assert.equal(seriesColor(-1, 'dark'), null);
  });
  it('adds alpha to hex colors only', () => {
    assert.equal(withAlpha('#2a78d6', 0.5), '#2a78d680');
    assert.equal(withAlpha('#2a78d6', 2), '#2a78d6ff');
    assert.equal(withAlpha('red', 0.5), 'red');
  });
  it('maps row tones to status colors', () => {
    assert.equal(statusOfTone('done'), 'good');
    assert.equal(statusOfTone('problem'), 'critical');
    assert.equal(statusOfTone('neutral'), null);
  });
});

describe('scale', () => {
  it('picks nice steps', () => {
    assert.equal(niceStep(0.7), 1);
    assert.equal(niceStep(1.3), 2);
    assert.equal(niceStep(23), 25);
    assert.equal(niceStep(420), 500);
    assert.equal(niceStep(0), 1);
  });
  it('builds ticks from zero past the max', () => {
    assert.deepEqual(niceTicks(0, 87), [0, 25, 50, 75, 100]);
    assert.deepEqual(niceTicks(3, 9, 3), [0, 5, 10]);
    assert.deepEqual(niceTicks(0, 0), [0, 0.25, 0.5, 0.75, 1]);
    assert.deepEqual(niceTicks(-10, 30, 4), [-10, 0, 10, 20, 30]);
    assert.deepEqual(niceTicks(0, 0.3, 3), [0, 0.1, 0.2, 0.3]);
  });
  it('maps linearly and survives a zero domain', () => {
    const y = linear(0, 100, 200, 0);
    assert.equal(y(0), 200);
    assert.equal(y(50), 100);
    assert.equal(linear(5, 5, 10, 0)(5), 10);
  });
  it('spreads points and bands', () => {
    assert.equal(pointX(0, 5, 100), 0);
    assert.equal(pointX(4, 5, 100), 100);
    assert.equal(pointX(0, 1, 100), 50);
    const b = bandLayout(4, 2, 100, 0.2);
    assert.equal(b.bucket, 25);
    assert.equal(b.bar, 10);
    assert.equal(b.x(1, 1), 25 + 2.5 + 10);
    assert.equal(b.center(0), 12.5);
  });
  it('finds the bucket under a touch', () => {
    assert.equal(nearestIndex(0, 5, 100, 'point'), 0);
    assert.equal(nearestIndex(60, 5, 100, 'point'), 2);
    assert.equal(nearestIndex(130, 5, 100, 'point'), 4);
    assert.equal(nearestIndex(99, 4, 100, 'band'), 3);
    assert.equal(nearestIndex(100, 4, 100, 'band'), 3);
    assert.equal(nearestIndex(10, 0, 100, 'band'), -1);
  });
  it('thins x labels and keeps both ends', () => {
    assert.deepEqual(labelIndices(3, 5), [0, 1, 2]);
    assert.deepEqual(labelIndices(30, 4), [0, 10, 19, 29]);
    assert.deepEqual(labelIndices(0, 4), []);
  });
  it('computes extents and paths with gaps', () => {
    assert.deepEqual(extent([3, null, -2, NaN, 8]), { min: -2, max: 8 });
    assert.deepEqual(extent([]), { min: 0, max: 0 });
    assert.equal(linePath([{ x: 0, y: 1 }, { x: 10, y: 2 }, null, { x: 30, y: 4 }]), 'M0,1L10,2M30,4');
  });
  it('splits a donut by share', () => {
    const a = donutAngles([1, 1, 0, 2]);
    assert.equal(a[0].start, 0);
    assert.ok(Math.abs(a[1].end - Math.PI) < 1e-9);
    assert.equal(a[2].end - a[2].start, 0);
    assert.ok(Math.abs(a[3].end - Math.PI * 2) < 1e-9);
    assert.equal(arcPath(50, 30, 40, 0, 0), '');
    assert.match(arcPath(50, 30, 40, 0, Math.PI * 2), /^M50,10 A40,40 0 1 1/);
  });
});

describe('delta', () => {
  it('computes the change and refuses a zero base', () => {
    assert.equal(deltaPct(120, 100), 20);
    assert.equal(deltaPct(50, 100), -50);
    assert.equal(deltaPct(5, 0), null);
    assert.equal(deltaPct(null, 10), null);
    assert.equal(deltaPct(-5, -10), 50);
  });
  it('reads direction and tone by the good side', () => {
    assert.equal(direction(0.2), 'flat');
    assert.equal(direction(null), null);
    assert.equal(deltaTone(12, 'up'), 'good');
    assert.equal(deltaTone(12, 'down'), 'bad');
    assert.equal(deltaTone(-12, 'down'), 'good');
    assert.equal(deltaTone(0.1, 'up'), 'neutral');
    assert.equal(deltaTone(null), 'neutral');
  });
  it('formats with an explicit sign', () => {
    assert.equal(formatDelta(12.345, 'en'), '+12 %');
    assert.equal(formatDelta(-3.25, 'en'), '−3.3 %');
    assert.equal(formatDelta(0.1, 'en'), '±0.1 %');
    assert.equal(formatDelta(4.5, 'tr'), '+4,5 %');
  });
});

describe('format', () => {
  it('compacts axis numbers', () => {
    assert.equal(compact(850, 'en'), '850');
    assert.equal(compact(1500, 'en'), '1.5K');
    assert.equal(compact(2.5, 'en'), '2.5');
  });
  it('formats by unit', () => {
    assert.equal(formatValue(null, 'count', 'en'), '–');
    assert.equal(formatValue(12, 'percent', 'en'), '12 %');
    assert.equal(formatValue(1234.5, 'money', 'en', 'USD'), '$1,235');
    assert.equal(formatValue(12.5, 'money', 'en', 'USD'), '$12.50');
    assert.equal(formatValue(1234, 'count', 'en'), '1,234');
  });
  it('labels buckets by granularity', () => {
    assert.equal(bucketLabel('2026-11-03', 'day', 'en'), 'Nov 3');
    assert.equal(bucketLabel('2026-11-01', 'month', 'en'), 'Nov 26');
    assert.equal(bucketLabel('2026-11-01', 'month', 'en', true), 'November 2026');
    assert.equal(bucketLabel('x', 'day', 'en'), 'x');
  });
  it('guesses granularity from keys', () => {
    assert.equal(guessGranularity(['2026-01-01', '2026-01-02']), 'day');
    assert.equal(guessGranularity(['2026-01-05', '2026-01-12']), 'week');
    assert.equal(guessGranularity(['2026-01-01', '2026-02-01']), 'month');
  });
});

describe('envelope', () => {
  const day = (k: string, v: number, previous?: number | null) => ({ key: k, label: k, value: v, previous });
  const created = { key: 'created', label: 'Created', unit: 'count', points: [day('2026-11-01', 2, 1), day('2026-11-02', 3, null)] };
  const completed = { key: 'completed', label: 'Completed', unit: 'count', points: [day('2026-11-01', 1), day('2026-11-02', 2)] };
  const meters = { key: 'meters', label: 'Meters', unit: 'meters', points: [day('2026-11-01', 10), day('2026-11-02', 20)] };
  const status = { key: 'status', label: 'Status', unit: 'count', points: [{ key: 'open', label: 'Open', value: 4 }] };

  it('tells time series apart', () => {
    assert.equal(isTimeSeries(created), true);
    assert.equal(isTimeSeries(status), false);
    assert.equal(hasPrevious([created]), true);
    assert.equal(hasPrevious([completed]), false);
  });
  it('groups series that can share one axis', () => {
    const g = groupTimeSeries([created, completed, meters, status]);
    assert.equal(g.length, 2);
    assert.deepEqual(
      g[0].map((s) => s.key),
      ['created', 'completed'],
    );
    assert.deepEqual(
      g[1].map((s) => s.key),
      ['meters'],
    );
  });
  it('totals a series, preferring the API total', () => {
    assert.equal(seriesTotal(created), 5);
    assert.equal(seriesTotal({ ...created, total: 9 }), 9);
  });
  it('reads KPIs from kpis[] or the legacy cards series', () => {
    const cards = { key: 'cards', label: 'Cards', unit: 'count', points: [{ key: 'services_open', label: 'Open', value: 3 }] };
    assert.deepEqual(kpisOf({ series: [cards] }), [{ key: 'services_open', label: 'Open', value: 3, previous: null }]);
    const k = [{ key: 'a', label: 'A', value: 1, previous: 2, delta_pct: -50, good: 'up' as const }];
    assert.equal(kpisOf({ series: [cards], kpis: k }), k);
    assert.deepEqual(
      chartSeries({ series: [cards, created, { ...completed, points: [] }] }).map((s) => s.key),
      ['created'],
    );
  });
  it('finds the previous range and row links', () => {
    assert.deepEqual(previousRange({ period: { previous_from: '2026-10-01', previous_to: '2026-10-31' } }), {
      from: '2026-10-01',
      to: '2026-10-31',
    });
    assert.equal(previousRange({ period: '30d' }), null);
    assert.deepEqual(rowLink({ _link: { kind: 'order', uuid: 'u1' } }), { kind: 'order', uuid: 'u1' });
    assert.equal(rowLink({ _link: { kind: 'order' } }), null);
    assert.deepEqual(rowStatus({ s: 'x', _status: { s: { label: 'Done', tone: 'done' } } }, 's'), { label: 'Done', tone: 'done' });
    assert.deepEqual(rowStatus({ s: 'open' }, 's'), { label: 'open', tone: 'neutral' });
    assert.equal(rowStatus({ s: null }, 's'), null);
  });
});

describe('table', () => {
  const cols: Column[] = [
    { key: 'name', label: 'Name', type: 'text' },
    { key: 'count', label: 'Count', type: 'integer' },
    { key: 'total', label: 'Total', type: 'money' },
    { key: 'rate', label: 'Rate', type: 'percent', aggregate: 'avg' },
    { key: 'at', label: 'At', type: 'datetime' },
  ];
  const rows = [
    { name: 'İzmir Oto', count: 3, total: '100.50', currency: 'TRY', rate: 10, at: '2026-11-01T10:00:00Z' },
    { name: 'ankara film', count: 7, total: '20.00', currency: 'USD', rate: 30, at: null },
    { name: 'Çorum', count: null, total: '9.50', currency: 'TRY', rate: null, at: '2026-11-03T10:00:00Z' },
  ];

  it('aligns numbers right', () => {
    assert.equal(alignOf(cols[0]), 'left');
    assert.equal(alignOf(cols[2]), 'right');
    assert.equal(alignOf({ ...cols[2], align: 'left' }), 'left');
  });
  it('cycles sort: first direction, the other, off', () => {
    assert.deepEqual(nextSort(null, cols[1]), { key: 'count', dir: 'desc' });
    assert.deepEqual(nextSort({ key: 'count', dir: 'desc' }, cols[1]), { key: 'count', dir: 'asc' });
    assert.equal(nextSort({ key: 'count', dir: 'asc' }, cols[1]), null);
    assert.deepEqual(nextSort({ key: 'count', dir: 'asc' }, cols[0]), { key: 'name', dir: 'asc' });
  });
  it('sorts numbers, money, dates and text with empties last', () => {
    const by = (key: string, dir: 'asc' | 'desc') => sortRows(rows, cols, { key, dir }).map((r) => r.name);
    assert.deepEqual(by('count', 'desc'), ['ankara film', 'İzmir Oto', 'Çorum']);
    assert.deepEqual(by('count', 'asc'), ['İzmir Oto', 'ankara film', 'Çorum']);
    assert.deepEqual(by('total', 'asc'), ['Çorum', 'ankara film', 'İzmir Oto']);
    assert.deepEqual(by('at', 'desc'), ['Çorum', 'İzmir Oto', 'ankara film']);
    assert.deepEqual(by('name', 'asc'), ['ankara film', 'Çorum', 'İzmir Oto']);
    assert.deepEqual(
      sortRows(rows, cols, null).map((r) => r.name),
      rows.map((r) => r.name),
    );
  });
  it('searches without case and accents', () => {
    assert.equal(fold('İZMİR Çorum'), 'izmir corum');
    assert.deepEqual(
      searchRows(rows, cols, 'izmir').map((r) => r.name),
      ['İzmir Oto'],
    );
    assert.deepEqual(
      searchRows(rows, cols, 'corum').map((r) => r.name),
      ['Çorum'],
    );
    assert.equal(searchRows(rows, cols, '  ').length, 3);
    assert.equal(searchRows(rows, cols, 'ankara oto').length, 0);
  });
  it('totals per column and money per currency', () => {
    const t = totals(rows, cols);
    assert.deepEqual(t.count, { kind: 'number', value: 10 });
    assert.deepEqual(t.total, { kind: 'money', byCurrency: { TRY: 110, USD: 20 } });
    assert.deepEqual(t.rate, { kind: 'number', value: 20 });
    assert.equal(t.name, undefined);
    assert.equal(t.at, undefined);
    assert.equal(aggregateOf({ key: 'x', label: 'x', type: 'number' }), 'none');
    assert.equal(aggregateOf({ key: 'x', label: 'x', type: 'number', aggregate: 'sum' }), 'sum');
  });
});

describe('period', () => {
  it('validates custom ranges', () => {
    assert.equal(rangeDays('2026-11-01', '2026-11-30'), 30);
    assert.equal(rangeError('2026-11-01', '2026-11-30'), null);
    assert.equal(rangeError('2026-11-30', '2026-11-01'), 'reversed');
    assert.equal(rangeError('2024-01-01', '2026-01-05'), 'too_long');
    assert.equal(rangeError(undefined, '2026-01-05'), 'invalid');
  });
  it('offers bucket sizes that fit the range', () => {
    assert.deepEqual(allowedGranularities(7), ['day']);
    assert.deepEqual(allowedGranularities(30), ['day', 'week']);
    assert.deepEqual(allowedGranularities(90), ['day', 'week', 'month']);
    assert.deepEqual(allowedGranularities(365), ['week', 'month']);
    assert.equal(effectiveGranularity({ ...DEFAULT_PERIOD, preset: '12m', granularity: 'day' }), null);
    assert.equal(effectiveGranularity({ ...DEFAULT_PERIOD, preset: '12m', granularity: 'month' }), 'month');
  });
  it('sends only what the report takes', () => {
    assert.deepEqual(toQuery(DEFAULT_PERIOD, { periodic: false }), {});
    assert.deepEqual(toQuery(DEFAULT_PERIOD, { periodic: true }), { period: '30d' });
    assert.deepEqual(toQuery(DEFAULT_PERIOD, { periodic: true, compare: true }), { period: '30d', compare: 'previous' });
    assert.deepEqual(
      toQuery({ preset: 'custom', from: '2026-10-01', to: '2026-10-31', granularity: 'week', compare: 'year' }, { periodic: true, granularity: true, compare: true }),
      { range_from: '2026-10-01', range_to: '2026-10-31', granularity: 'week', compare: 'year' },
    );
    assert.deepEqual(toQuery({ preset: 'custom', from: '2026-10-31', to: '2026-10-01', granularity: null, compare: 'none' }, { periodic: true }), {
      period: '30d',
    });
  });
  it('builds a month grid starting on Monday', () => {
    const g = monthGrid(2026, 10); // November 2026 starts on a Sunday
    assert.equal(g[0][0].iso, '2026-10-26');
    assert.equal(g[0][6].iso, '2026-11-01');
    assert.equal(g[0][6].inMonth, true);
    assert.equal(g[g.length - 1][6].iso, '2026-12-06');
    assert.ok(g.every((w) => w.length === 7));
    assert.equal(monthGrid(2026, 10, 0)[0][0].iso, '2026-11-01');
  });
  it('picks a range with two taps', () => {
    assert.deepEqual(tapRange({}, '2026-11-10'), { from: '2026-11-10' });
    assert.deepEqual(tapRange({ from: '2026-11-10' }, '2026-11-03'), { from: '2026-11-03', to: '2026-11-10' });
    assert.deepEqual(tapRange({ from: '2026-11-03', to: '2026-11-10' }, '2026-11-20'), { from: '2026-11-20' });
  });
});

describe('links', async () => {
  const { linkPath } = await import('./links.ts');
  it('opens records inside the current space', () => {
    assert.equal(linkPath('bayi', 'order', 'u-1'), '/bayi/menu/orders/u-1');
    assert.equal(linkPath('admin', 'organization', 'u-1'), '/admin/network/u-1');
    assert.equal(linkPath('bayi', 'organization', 'u-1'), '/bayi/menu/network/u-1');
    assert.equal(linkPath('bayi', 'service', 'u-1'), '/bayi/menu/customers/services/u-1');
    assert.equal(linkPath('admin', 'product', 'u-1'), null);
    assert.equal(linkPath('depo', 'order', 'u-1'), '/depo/menu/orders/u-1');
    assert.equal(linkPath('depo', 'customer', 'u-1'), null);
    assert.equal(linkPath('musteri', 'order', 'u-1'), null);
    assert.equal(linkPath('bayi', 'unknown', 'u-1'), null);
    assert.equal(linkPath('bayi', 'order', '../x'), null);
  });
});
