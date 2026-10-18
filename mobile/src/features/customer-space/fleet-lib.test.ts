import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { fleetLinkTone, fleetWarrantyTone, reportPeriod, sortLinks } from './fleet-lib.ts';

const link = (uuid: string, status: 'pending' | 'active' | 'ended') =>
  ({ uuid, status, dealer_uuid: 'd', dealer_name: 'D', started_at: null, ended_at: null, created_at: '' }) as const;

describe('fleet tones', () => {
  it('maps warranty states and link statuses', () => {
    assert.equal(fleetWarrantyTone('active'), 'done');
    assert.equal(fleetWarrantyTone('void'), 'problem');
    assert.equal(fleetWarrantyTone('expired'), 'neutral');
    assert.equal(fleetLinkTone('pending'), 'progress');
    assert.equal(fleetLinkTone('ended'), 'neutral');
  });
});

describe('sortLinks', () => {
  it('puts pending requests first without mutating the input', () => {
    const input = [link('a', 'ended'), link('b', 'active'), link('c', 'pending')];
    assert.deepEqual(
      sortLinks(input).map((l) => l.uuid),
      ['c', 'b', 'a'],
    );
    assert.equal(input[0].uuid, 'a');
  });
});

describe('reportPeriod', () => {
  it('cuts to the day', () => {
    assert.equal(reportPeriod('2026-07-01T00:00:00Z', '2026-09-30'), '2026-07-01 – 2026-09-30');
  });
});
