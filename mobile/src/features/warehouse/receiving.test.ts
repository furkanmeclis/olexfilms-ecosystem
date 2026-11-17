import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import type { StockEntry, StockEntryArrivalReport } from './api.ts';
import { arrivalChecked, buildDecisions, isIsoDate, timelineDetail, unknownExtras } from './receiving.ts';

const line = (barcode: string, scanned = false) =>
  ({
    uuid: barcode,
    barcode,
    scanned_at: scanned ? '2026-11-17T10:00:00Z' : null,
    product: { name: 'Film' },
  }) as unknown as NonNullable<StockEntry['lines']>[number];

const report = {
  expected: 3,
  scanned: 1,
  missing: [line('B'), line('C')],
  extras: [
    {
      barcode: 'X',
      product: { uuid: 'p', sku: 'S', name: 'Film' },
      scanned_at: '',
    },
    { barcode: 'Z', product: null, scanned_at: '' },
  ],
} as unknown as StockEntryArrivalReport;

describe('TEC-1225 mobile receiving', () => {
  it('applies the arrival check to container and scanned entries', () => {
    const base = { lines: [line('A')] } as unknown as StockEntry;
    assert.equal(arrivalChecked(base), false);
    assert.equal(arrivalChecked({ ...base, container_no: 'MSCU1' }), true);
    assert.equal(arrivalChecked({ ...base, lines: [line('A', true)] }), true);
    assert.equal(arrivalChecked(base, report), true);
  });

  it('builds the decisions of the record', () => {
    assert.deepEqual(buildDecisions(report, true, {}, []).pending, ['X', 'Z']);
    const done = buildDecisions(report, true, { X: 'added', Z: 'rejected' }, [
      {
        barcode: 'C',
        decision: 'excluded',
        reason: 'ezik',
        attachmentUuids: ['ph'],
      },
    ]);
    assert.deepEqual(done.pending, []);
    assert.deepEqual(done.decisions, [
      { barcode: 'B', kind: 'missing', decision: 'left_out' },
      { barcode: 'X', kind: 'extra', decision: 'added' },
      { barcode: 'Z', kind: 'extra', decision: 'rejected' },
      {
        barcode: 'C',
        kind: 'damaged',
        decision: 'excluded',
        reason: 'ezik',
        attachment_uuids: ['ph'],
      },
    ]);
    assert.deepEqual(
      buildDecisions(report, false, { X: 'added', Z: 'rejected' }, [
        { barcode: 'A', decision: 'accepted', reason: '', attachmentUuids: [] },
      ]).pending,
      ['A'],
    );
  });

  it('validates dates, unknown extras and timeline details', () => {
    assert.equal(isIsoDate('2026-11-17'), true);
    assert.equal(isIsoDate('2026-02-30'), false);
    assert.equal(isIsoDate('17.11.2026'), false);
    assert.deepEqual([...unknownExtras(report)], ['Z']);
    assert.equal(
      timelineDetail({ barcode: 'A', count: 3 }, (k) => k),
      'A · ×3',
    );
    assert.equal(
      timelineDetail({ kind: 'photo', filename: 'x.jpg' }, (k) => `K:${k}`),
      'K:photo · x.jpg',
    );
  });
});
