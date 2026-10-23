import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  type AIActionCard,
  applyStreamEvent,
  type ChatMessage,
  editsToSend,
  formatCountdown,
  fromServerMessage,
  initialEdits,
  messageText,
  missingRequired,
  quotaRemainingPercent,
  quotaUsedUp,
  recordPath,
  secondsUntil,
  type StreamEvent,
} from './chat-state.ts';
import { actionLabel, chatErrorLabel, fieldLabel, toolLabel, warningLabel } from './labels.ts';

const card: AIActionCard = {
  action_uuid: 'a1',
  tool_use_id: 'tu',
  tool_name: 'create_task',
  source: 'panel',
  status: 'pending',
  preview: { action: 'create_task' },
  expires_at: '2026-01-01T00:10:00Z',
  created_at: '2026-01-01T00:00:00Z',
};

function run(events: StreamEvent[]) {
  let messages: ChatMessage[] = [{ id: 'local', role: 'assistant', status: 'pending', blocks: [] }];
  let id = 'local';
  for (const e of events) ({ messages, id } = applyStreamEvent(messages, id, e));
  return { messages, id };
}

describe('applyStreamEvent', () => {
  it('swaps the local id and joins text deltas', () => {
    const { messages, id } = run([
      { event: 'message_start', data: { conversation_uuid: 'c', message_uuid: 'm1' } },
      { event: 'text_delta', data: { text: 'Mer' } },
      { event: 'text_delta', data: { text: 'haba' } },
      { event: 'message_done', data: { message_uuid: 'm1', status: 'complete' } },
    ]);
    assert.equal(id, 'm1');
    assert.equal(messages[0].id, 'm1');
    assert.equal(messages[0].status, 'complete');
    assert.equal(messages[0].blocks.length, 1);
    assert.equal(messageText(messages[0]), 'Merhaba');
  });

  it('tracks tools and splits text around them', () => {
    const { messages } = run([
      { event: 'text_delta', data: { text: 'a' } },
      { event: 'tool_start', data: { id: 't1', name: 'my_tasks' } },
      { event: 'tool_result', data: { id: 't1', name: 'my_tasks', ok: false } },
      { event: 'text_delta', data: { text: 'b' } },
    ]);
    assert.deepEqual(
      messages[0].blocks.map((b) => b.type),
      ['text', 'tool', 'text'],
    );
    assert.deepEqual(messages[0].blocks[1], { type: 'tool', id: 't1', name: 'my_tasks', status: 'error' });
  });

  it('closes the confirmed card wherever it is', () => {
    let { messages } = run([{ event: 'confirm', data: card }]);
    messages = [...messages, { id: 'next', role: 'assistant', status: 'pending', blocks: [] }];
    const out = applyStreamEvent(messages, 'next', {
      event: 'action',
      data: { action_uuid: 'a1', tool_use_id: 'tu', tool_name: 'create_task', source: 'panel', status: 'confirmed' },
    }).messages;
    const first = out[0].blocks[0];
    assert.equal(first.type === 'confirm' && first.card.status, 'confirmed');
    assert.equal(out[1].blocks[0].type, 'action');
  });

  it('marks errors and cancellations', () => {
    assert.equal(run([{ event: 'error', data: { code: 'provider_error' } }]).messages[0].status, 'error');
    assert.equal(
      run([{ event: 'message_done', data: { message_uuid: 'x', status: 'cancelled' } }]).messages[0].status,
      'cancelled',
    );
  });

  it('ignores events it does not change the message for', () => {
    const before = run([]).messages;
    assert.deepEqual(run([{ event: 'title', data: { conversation_uuid: 'c', title: 't' } }]).messages, before);
  });
});

describe('fromServerMessage', () => {
  it('maps stored blocks', () => {
    const m = fromServerMessage({
      uuid: 'u',
      role: 'assistant',
      status: 'complete',
      created_at: '',
      ui: [
        { type: 'text', text: 'hi' },
        { type: 'text', text: '' },
        { type: 'tool', id: 't', name: 'x', status: 'weird' },
        { type: 'confirm', card },
        { type: 'error', code: 'refusal' },
      ],
    });
    assert.deepEqual(
      m.blocks.map((b) => b.type),
      ['text', 'tool', 'confirm', 'error'],
    );
    assert.equal(m.blocks[1].type === 'tool' && m.blocks[1].status, 'running');
  });
});

describe('quota and countdown', () => {
  it('computes the remaining percent', () => {
    assert.equal(quotaRemainingPercent({ limit: 0, used: 5 }), null);
    assert.equal(quotaRemainingPercent({ limit: 200, used: 50 }), 75);
    assert.equal(quotaRemainingPercent({ limit: 10, used: 20 }), 0);
  });
  it('knows when the quota is used up', () => {
    assert.equal(quotaUsedUp(undefined), false);
    assert.equal(quotaUsedUp({ period: 'p', limit: 0, used: 9, remaining: null }), false);
    assert.equal(quotaUsedUp({ period: 'p', limit: 10, used: 10, remaining: 0 }), true);
  });
  it('formats the countdown', () => {
    assert.equal(secondsUntil('2026-01-01T00:01:00Z', Date.parse('2026-01-01T00:00:00Z')), 60);
    assert.equal(secondsUntil('2026-01-01T00:00:00Z', Date.parse('2026-01-01T00:05:00Z')), 0);
    assert.equal(secondsUntil('nope', 0), 0);
    assert.equal(formatCountdown(65), '1:05');
    assert.equal(formatCountdown(3725), '1:02:05');
  });
});

describe('card edits', () => {
  const preview = {
    edit: [
      { key: 'title', type: 'text' as const, value: 'Ara', required: true },
      { key: 'estimated_minutes', type: 'number' as const, value: '30' },
    ],
  };
  it('starts from the proposed values', () => {
    assert.deepEqual(initialEdits(preview), { title: 'Ara', estimated_minutes: '30' });
  });
  it('blocks an empty required field', () => {
    assert.equal(missingRequired(preview, { title: ' ', estimated_minutes: '' }), true);
    assert.equal(missingRequired(preview, { title: 'x', estimated_minutes: '' }), false);
  });
  it('sends numbers as numbers and nothing without fields', () => {
    assert.deepEqual(editsToSend(preview, { title: ' x ', estimated_minutes: '45' }), { title: 'x', estimated_minutes: 45 });
    assert.equal(editsToSend({}, {}), undefined);
  });
});

describe('labels and links', () => {
  const t = (k: string) => `t:${k}`;
  it('labels known names and falls back otherwise', () => {
    assert.equal(toolLabel(t, 'my_tasks'), 't:tools.my_tasks');
    assert.equal(toolLabel(t, 'new_tool'), 't:chat.tool_generic');
    assert.equal(actionLabel(t, 'create_task'), 't:actions.create_task');
    assert.equal(actionLabel(t, 'other', 'Özet'), 'Özet');
    assert.equal(actionLabel(t, 'other'), 't:card.title');
    assert.equal(fieldLabel(t, 'plate'), 't:actions.fields.plate');
    assert.equal(fieldLabel(t, 'raw_key'), 'raw_key');
    assert.equal(warningLabel(t, 'order_draft_not_submitted'), 't:actions.warnings.order_draft_not_submitted');
    assert.equal(warningLabel(t, 'x'), null);
    assert.equal(chatErrorLabel(t, 'network'), 't:errors.network');
    assert.equal(chatErrorLabel(t, 'x', 'server says'), 'server says');
    assert.equal(chatErrorLabel(t, 'x'), 't:errors.generic');
  });
  it('links records that have a mobile screen', () => {
    assert.equal(recordPath('bayi', 'lead', 'u'), '/bayi/menu/leads/u');
    assert.equal(recordPath('admin', 'task', 'u'), '/admin/menu/tasks/u');
    assert.equal(recordPath('bayi', 'task', 'u'), null);
    assert.equal(recordPath('bayi', 'appointment', 'u'), null);
  });
});
