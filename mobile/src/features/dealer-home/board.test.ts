import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  BOARD_GROUPS,
  boardRows,
  fitTags,
  groupOf,
  groupServices,
  kpiIcon,
  listFilterOf,
  sortForGroup,
  statusParam,
  uniqueCategories,
} from './board.ts';

const row = (uuid: string, status: string, created_at: string, completed_at: string | null = null) => ({
  uuid,
  status,
  created_at,
  completed_at,
});

describe('groupOf', () => {
  it('maps statuses to the three groups', () => {
    assert.equal(groupOf('draft'), 'next');
    assert.equal(groupOf('pending'), 'next');
    assert.equal(groupOf('processing'), 'active');
    assert.equal(groupOf('ready'), 'active');
    assert.equal(groupOf('completed'), 'done');
  });

  it('leaves cancelled and unknown statuses off the board', () => {
    assert.equal(groupOf('cancelled'), undefined);
    assert.equal(groupOf('whatever'), undefined);
  });
});

describe('statusParam / listFilterOf', () => {
  it('joins statuses for the API and filters the list only for one status', () => {
    const [next, active, done] = BOARD_GROUPS;
    assert.equal(statusParam(next), 'draft,pending');
    assert.equal(statusParam(active), 'processing,ready');
    assert.equal(listFilterOf(next), undefined);
    assert.equal(listFilterOf(done), 'completed');
  });
});

describe('sortForGroup', () => {
  it('puts the oldest job first', () => {
    const rows = [row('b', 'pending', '2026-10-03T10:00:00Z'), row('a', 'draft', '2026-10-01T10:00:00Z')];
    assert.deepEqual(
      sortForGroup(rows, 'oldest').map((r) => r.uuid),
      ['a', 'b'],
    );
  });

  it('puts the latest completion first, falling back to created_at', () => {
    const rows = [
      row('a', 'completed', '2026-09-01T00:00:00Z', '2026-10-01T00:00:00Z'),
      row('b', 'completed', '2026-09-02T00:00:00Z', '2026-10-05T00:00:00Z'),
      row('c', 'completed', '2026-10-03T00:00:00Z', null),
    ];
    assert.deepEqual(
      sortForGroup(rows, 'newest').map((r) => r.uuid),
      ['b', 'c', 'a'],
    );
  });

  it('breaks ties by uuid and does not mutate the input', () => {
    const rows = [row('z', 'draft', '2026-10-01T00:00:00Z'), row('y', 'draft', '2026-10-01T00:00:00Z')];
    assert.deepEqual(
      sortForGroup(rows, 'oldest').map((r) => r.uuid),
      ['y', 'z'],
    );
    assert.equal(rows[0].uuid, 'z');
  });
});

describe('groupServices / boardRows', () => {
  it('groups, orders and drops cancelled rows', () => {
    const g = groupServices([
      row('p2', 'pending', '2026-10-05T00:00:00Z'),
      row('d1', 'draft', '2026-10-01T00:00:00Z'),
      row('r1', 'ready', '2026-10-02T00:00:00Z'),
      row('x1', 'cancelled', '2026-10-01T00:00:00Z'),
      row('c1', 'completed', '2026-09-01T00:00:00Z', '2026-10-06T00:00:00Z'),
    ]);
    assert.deepEqual(
      g.next.map((r) => r.uuid),
      ['d1', 'p2'],
    );
    assert.deepEqual(
      g.active.map((r) => r.uuid),
      ['r1'],
    );
    assert.deepEqual(
      g.done.map((r) => r.uuid),
      ['c1'],
    );
  });

  it('keeps at most the group limit', () => {
    const done = BOARD_GROUPS[2];
    const rows = Array.from({ length: 10 }, (_, i) =>
      row(`c${i}`, 'completed', '2026-09-01T00:00:00Z', `2026-10-${String(i + 1).padStart(2, '0')}T00:00:00Z`),
    );
    const out = boardRows(rows, done);
    assert.equal(out.length, done.limit);
    assert.equal(out[0].uuid, 'c9');
  });
});

describe('uniqueCategories', () => {
  it('lists each category once, first spelling kept, in API order', () => {
    const names = uniqueCategories({ item_categories: ['PPF', ' ppf ', 'Cam filmi', '', '  ', null] });
    assert.deepEqual(names, ['PPF', 'Cam filmi']);
  });

  it('is empty for an empty list', () => {
    assert.deepEqual(uniqueCategories({ item_categories: [] }), []);
  });

  it('shows no tags when an older backend omits the field', () => {
    assert.deepEqual(uniqueCategories({}), []);
    assert.deepEqual(uniqueCategories({ item_categories: undefined }), []);
    assert.deepEqual(uniqueCategories({ item_categories: null }), []);
    // Not an array at all (unexpected payload): still no crash.
    assert.deepEqual(uniqueCategories({ item_categories: 'PPF' as unknown as string[] }), []);
  });

  it('ignores the old items[].product.category path', () => {
    const legacy = { items: [{ product: { category: { name: 'PPF' } } }] };
    assert.deepEqual(uniqueCategories(legacy as unknown as { item_categories?: string[] }), []);
  });

  it('keeps three chips with +N for many categories', () => {
    const tags = fitTags(uniqueCategories({ item_categories: ['PPF', 'Cam filmi', 'Seramik', 'Boya koruma'] }));
    assert.deepEqual(tags, { shown: ['PPF', 'Cam filmi'], more: 2 });
  });
});

describe('fitTags', () => {
  it('shows all tags that fit', () => {
    assert.deepEqual(fitTags(['a', 'b', 'c'], 3), { shown: ['a', 'b', 'c'], more: 0 });
  });

  it('folds the rest into +N without exceeding max chips', () => {
    assert.deepEqual(fitTags(['a', 'b', 'c', 'd', 'e'], 3), { shown: ['a', 'b'], more: 3 });
  });
});

describe('kpiIcon', () => {
  it('maps known card keys', () => {
    assert.equal(kpiIcon('services_created'), 'service');
    assert.equal(kpiIcon('customers_total'), 'customers');
    assert.equal(kpiIcon('warranties_active'), 'warranty');
  });

  it('falls back to a neutral icon for unknown keys', () => {
    assert.equal(kpiIcon('brand_new_card'), 'chart');
    assert.equal(kpiIcon('toString'), 'chart');
  });
});
