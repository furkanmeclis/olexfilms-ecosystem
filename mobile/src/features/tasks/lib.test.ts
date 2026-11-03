import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  dueDateToIso,
  isOverdue,
  nextStatuses,
  priorityTone,
  quickDueDate,
  statusQuery,
  statusTone,
  taskFormMissing,
} from './lib.ts';

describe('task lib', () => {
  it('maps tones', () => {
    assert.equal(statusTone('done'), 'done');
    assert.equal(statusTone('cancelled'), 'neutral');
    assert.equal(statusTone('open'), 'progress');
    assert.equal(priorityTone('urgent'), 'problem');
    assert.equal(priorityTone('low'), 'neutral');
  });

  it('detects overdue only for open tasks', () => {
    const now = new Date('2026-10-19T12:00:00Z');
    assert.equal(isOverdue({ status: 'open', due_at: '2026-10-18T00:00:00Z' }, now), true);
    assert.equal(isOverdue({ status: 'done', due_at: '2026-10-18T00:00:00Z' }, now), false);
    assert.equal(isOverdue({ status: 'open', due_at: null }, now), false);
    assert.equal(isOverdue({ status: 'in_progress', due_at: '2026-10-20T00:00:00Z' }, now), false);
  });

  it('never offers the current status', () => {
    for (const s of ['open', 'in_progress', 'done', 'cancelled'] as const) {
      assert.ok(!nextStatuses(s).includes(s));
      assert.ok(nextStatuses(s).length > 0);
    }
  });

  it('builds the status query', () => {
    assert.equal(statusQuery('all'), undefined);
    assert.equal(statusQuery('active'), 'active');
  });

  it('parses due dates', () => {
    assert.equal(dueDateToIso(''), null);
    assert.equal(dueDateToIso('nope'), undefined);
    assert.equal(typeof dueDateToIso('2026-10-25'), 'string');
  });

  it('fills quick due dates across a month end', () => {
    const now = new Date(2026, 9, 31, 22, 15);
    assert.equal(quickDueDate('today', now), '2026-10-31');
    assert.equal(quickDueDate('tomorrow', now), '2026-11-01');
    assert.equal(quickDueDate('week', now), '2026-11-07');
    assert.equal(typeof dueDateToIso(quickDueDate('week', now)), 'string');
  });

  it('lists what the new task still needs', () => {
    assert.deepEqual(taskFormMissing({ title: '', subject: false, due: '' }), ['title', 'subject']);
    assert.deepEqual(taskFormMissing({ title: 'Visit', subject: true, due: '12.11' }), ['due']);
    assert.deepEqual(taskFormMissing({ title: 'Visit', subject: true, due: '' }), []);
  });
});
