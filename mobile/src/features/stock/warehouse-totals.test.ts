import assert from 'node:assert/strict';
import { describe, it, test } from 'node:test';

import { balanceParts, filterTotals, inStockTotals, totalsTruncated, WAREHOUSE_TOTALS_MAX_PAGES, WAREHOUSE_TOTALS_PAGE, warehouseProductTotals } from './warehouse-totals.ts';

const row = (uuid: string, name: string, quantity: number, meters = '0.00', unitType = 'piece', sku = `SKU-${uuid}`) => ({
  product: { uuid, sku, name, unit_type: unitType, category: { name: 'Kat' } },
  quantity,
  meters,
});

describe('warehouseProductTotals', () => {
  it('takes the quantity from the API, so a fixed barcode with many pieces is not counted as 1', () => {
    const rows = warehouseProductTotals([row('a', 'Ragle', 40), row('b', 'Seramik', 2)]);
    assert.deepEqual(
      rows.map((r) => [r.product.name, r.units, r.meters, r.category]),
      [
        ['Ragle', 40, null, 'Kat'],
        ['Seramik', 2, null, 'Kat'],
      ],
    );
  });

  it('reads the meters of roll products', () => {
    const [r] = warehouseProductTotals([row('r', 'PPF', 2, '15.75', 'roll_meter')]);
    assert.equal(r.units, 2);
    assert.equal(r.meters, 15.75);
  });

  it('returns nothing for an empty warehouse', () => {
    assert.deepEqual(warehouseProductTotals([]), []);
  });
});

describe('inStockTotals', () => {
  it('drops rows with no pieces and no meters', () => {
    const rows = warehouseProductTotals([row('a', 'A', 0), row('b', 'B', 3), row('r', 'R', 0, '1.50', 'roll_meter')]);
    assert.deepEqual(inStockTotals(rows).map((r) => r.product.uuid), ['b', 'r']);
  });
});

describe('filterTotals', () => {
  const rows = warehouseProductTotals([row('a', 'Antrasit Film', 1, '0', 'piece', 'ANT-1'), row('b', 'Seramik', 1, '0', 'piece', 'SER-9')]);
  it('matches name or SKU, any case', () => {
    assert.deepEqual(filterTotals(rows, 'film').map((r) => r.product.uuid), ['a']);
    assert.deepEqual(filterTotals(rows, 'ser-9').map((r) => r.product.uuid), ['b']);
  });
  it('keeps every row without a search', () => {
    assert.equal(filterTotals(rows, '  ').length, 2);
  });
});

test('TEC-1229: balance split of a warehouse row and its line', () => {
  const [row] = warehouseProductTotals([
    {
      product: { uuid: 'p', sku: 'S', name: 'N', unit_type: 'piece' },
      quantity: 3,
      meters: '0.00',
      balance: {
        reserved_quantity: 1,
        available_quantity: 2,
        incoming_quantity: 4,
        quarantine_quantity: 0,
        available_meters: '0.00',
        reserved_meters: '0.00',
      },
    },
  ]);
  assert.deepEqual([row.reserved, row.available, row.incoming], [1, 2, 4]);
  assert.deepEqual(balanceParts(row), [
    { key: 'm.balance_available', count: 2 },
    { key: 'm.balance_reserved', count: 1 },
    { key: 'm.balance_incoming', count: 4 },
  ]);
  assert.deepEqual(balanceParts({ reserved: 0, available: 3, incoming: 0 }), []);
});

describe('totalsTruncated (TEC-1224)', () => {
  const cap = WAREHOUSE_TOTALS_PAGE * WAREHOUSE_TOTALS_MAX_PAGES;
  it('flags a warehouse with more rows than were read', () => {
    assert.equal(cap, 5000);
    assert.equal(totalsTruncated(cap, cap + 1), true);
  });
  it('is quiet when every row was read or the total is unknown', () => {
    assert.equal(totalsTruncated(cap, cap), false);
    assert.equal(totalsTruncated(12, 12), false);
    assert.equal(totalsTruncated(0, undefined), false);
  });
});
