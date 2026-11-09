import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { filterTotals, totalsTruncated, warehouseProductTotals } from './warehouse-totals.ts';

const unit = (uuid: string, name: string, meters: string | null = null, sku = `SKU-${uuid}`) => ({
  product: { uuid, sku, name },
  remaining_meters: meters,
});

describe('warehouseProductTotals', () => {
  it('counts units per product, ordered by name', () => {
    const rows = warehouseProductTotals([unit('b', 'Seramik'), unit('a', 'Antrasit'), unit('b', 'Seramik')]);
    assert.deepEqual(
      rows.map((r) => [r.product.name, r.units, r.meters]),
      [
        ['Antrasit', 1, null],
        ['Seramik', 2, null],
      ],
    );
  });

  it('sums the meters left on rolls', () => {
    const [row] = warehouseProductTotals([unit('r', 'PPF', '12.5'), unit('r', 'PPF', '3.25')]);
    assert.equal(row.units, 2);
    assert.equal(row.meters, 15.75);
  });

  it('returns nothing for an empty warehouse', () => {
    assert.deepEqual(warehouseProductTotals([]), []);
  });
});

describe('filterTotals', () => {
  const rows = warehouseProductTotals([unit('a', 'Antrasit Film', null, 'ANT-1'), unit('b', 'Seramik', null, 'SER-9')]);
  it('matches name or SKU, any case', () => {
    assert.deepEqual(filterTotals(rows, 'film').map((r) => r.product.uuid), ['a']);
    assert.deepEqual(filterTotals(rows, 'ser-9').map((r) => r.product.uuid), ['b']);
  });
  it('keeps every row without a search', () => {
    assert.equal(filterTotals(rows, '  ').length, 2);
  });
});

describe('totalsTruncated', () => {
  it('is true only when the warehouse holds more units than were read', () => {
    assert.equal(totalsTruncated(1000, 1200), true);
    assert.equal(totalsTruncated(40, 40), false);
    assert.equal(totalsTruncated(0, undefined), false);
  });
});
