import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { aggregateOccupancy, occupancyLevel, occupancyPercent, occupancyRate, summarizeOccupancy } from './lib.ts';

const row = (uuid: string, name: string, capacity: number, occupied: number) => ({
  organization_uuid: uuid,
  organization_name: name,
  capacity,
  occupied,
  remaining: Math.max(capacity - occupied, 0),
});

describe('occupancy levels', () => {
  it('uses the web thresholds', () => {
    assert.equal(occupancyLevel(occupancyRate(0, 0)), 'none');
    assert.equal(occupancyLevel(occupancyRate(4, 10)), 'low');
    assert.equal(occupancyLevel(occupancyRate(5, 10)), 'medium');
    assert.equal(occupancyLevel(occupancyRate(8, 10)), 'high');
    assert.equal(occupancyLevel(occupancyRate(10, 10)), 'full');
    assert.equal(occupancyPercent(0.456), 46);
    assert.equal(occupancyPercent(null), null);
  });
});

describe('aggregateOccupancy', () => {
  it('sums a week per organization and sorts the busiest first', () => {
    const rows = aggregateOccupancy([
      [row('a', 'Alfa', 10, 2), row('b', 'Beta', 10, 9)],
      [row('a', 'Alfa', 10, 4), row('b', 'Beta', 10, 9)],
    ]);
    assert.deepEqual(rows.map((r) => r.organization_uuid), ['b', 'a']);
    assert.equal(rows[1]?.capacity, 20);
    assert.equal(rows[1]?.occupied, 6);
    assert.equal(rows[0]?.level, 'high');
  });
  it('puts organizations without capacity last', () => {
    const rows = aggregateOccupancy([[row('a', 'Alfa', 0, 0), row('b', 'Beta', 10, 1)]]);
    assert.deepEqual(rows.map((r) => r.organization_uuid), ['b', 'a']);
    assert.equal(rows[1]?.level, 'none');
  });
  it('summarizes the network', () => {
    const s = summarizeOccupancy(aggregateOccupancy([[row('a', 'A', 10, 5), row('b', 'B', 10, 10)]]));
    assert.equal(s.capacity, 20);
    assert.equal(s.occupied, 15);
    assert.equal(s.level, 'medium');
  });
});
