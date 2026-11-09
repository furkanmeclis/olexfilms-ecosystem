import assert from 'node:assert/strict';
import { test } from 'node:test';

import { pickWarehouse, sameWarehouse, stockKeys, transferFilter, warehouseFilter, wk } from './scope.ts';

const A = { uuid: 'wh-a', is_primary: false };
const B = { uuid: 'wh-b', is_primary: true };
const C = { uuid: 'wh-c' };

test('pickWarehouse: saved one while it is active, else primary, else first', () => {
  assert.equal(pickWarehouse([A, B, C], 'wh-c'), C);
  assert.equal(pickWarehouse([A, B, C], 'gone'), B);
  assert.equal(pickWarehouse([A, B, C], undefined), B);
  assert.equal(pickWarehouse([A, C], undefined), A);
  assert.equal(pickWarehouse([], 'wh-a'), undefined);
});

test('TEC-1049: scoped query keys differ per warehouse (a switch never reuses the old cache)', () => {
  assert.notDeepEqual(wk.counts('draft', 'wh-a'), wk.counts('draft', 'wh-b'));
  assert.notDeepEqual(wk.entries('draft', 'wh-a'), wk.entries('draft', 'wh-b'));
  assert.notDeepEqual(wk.transfers('draft', 'wh-a', 'out'), wk.transfers('draft', 'wh-b', 'out'));
  assert.notDeepEqual(wk.transfers('draft', 'wh-a', 'out'), wk.transfers('draft', 'wh-a', 'in'));
  assert.notDeepEqual(wk.summary('wh-a'), wk.summary('wh-b'));
  assert.notDeepEqual(stockKeys.warehouseUnits('wh-a', ''), stockKeys.warehouseUnits('wh-b', ''));
});

test('scoped keys keep the prefixes the write paths invalidate', () => {
  const prefix = (key: readonly unknown[], p: readonly unknown[]) => p.every((v, i) => key[i] === v);
  assert.ok(prefix(wk.counts('draft', 'wh-a'), ['warehouse', 'counts']));
  assert.ok(prefix(wk.entries('draft', 'wh-a'), ['warehouse', 'entries']));
  assert.ok(prefix(wk.transfers('draft', 'wh-a'), ['warehouse', 'transfers']));
  assert.ok(prefix(wk.summary('wh-a'), wk.summaries()));
  assert.ok(prefix(stockKeys.warehouseUnits('wh-a', 'q'), ['stock']));
});

test('sameWarehouse: placeholder rows only from the same warehouse', () => {
  assert.equal(sameWarehouse(wk.counts('draft', 'wh-a'), 'wh-a'), true);
  assert.equal(sameWarehouse(wk.counts('draft', 'wh-a'), 'wh-b'), false);
  assert.equal(sameWarehouse(stockKeys.warehouseUnits('wh-a', 'x'), 'wh-a'), true);
  assert.equal(sameWarehouse(stockKeys.warehouseUnits('wh-a', 'x'), 'wh-b'), false);
  assert.equal(sameWarehouse(undefined, 'wh-a'), false);
});

test('filters: the API params carry the picked warehouse', () => {
  assert.deepEqual(warehouseFilter('wh-a'), { warehouse_uuid: 'wh-a' });
  assert.deepEqual(warehouseFilter(undefined), {});
  assert.deepEqual(transferFilter('wh-a', 'out'), { from_warehouse_uuid: 'wh-a' });
  assert.deepEqual(transferFilter('wh-a', 'in'), { to_warehouse_uuid: 'wh-a' });
  assert.deepEqual(transferFilter(undefined, 'in'), {});
});
