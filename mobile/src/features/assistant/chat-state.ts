import type { components } from '@/core/api/schema';

/**
 * Chat model of the panel assistant (same rules as the web's
 * features/ai-assistant/lib/chat-state.ts). Pure: no React Native import.
 */
type S = components['schemas'];
export type AIStatus = S['AIStatus'];
export type AIConversation = S['AIConversation'];
export type AIMessage = S['AIMessage'];
export type AIActionCard = S['AIActionCard'];
export type AIActionOutcome = S['AIActionOutcome'];
export type LegalText = S['LegalText'];

/** `preview` of an action card (TEC-387 tools.Preview). */
export type ActionPreview = {
  action?: string;
  summary?: string;
  /** `code` is the raw value of an enum field; `value` is localized. */
  fields?: { key: string; value: string; code?: string }[];
  edit?: {
    key: string;
    type: 'text' | 'textarea' | 'date' | 'time' | 'select' | 'number';
    value: string;
    options?: string[];
    option_labels?: Record<string, string>;
    required?: boolean;
  }[];
  warnings?: string[];
};

export type ToolStatus = 'running' | 'done' | 'error' | 'pending';

export type ChatBlock =
  | { type: 'text'; text: string }
  | { type: 'tool'; id: string; name: string; status: ToolStatus }
  | { type: 'confirm'; card: AIActionCard }
  | { type: 'action'; action: AIActionOutcome }
  | { type: 'error'; code: string; message?: string };

export type ChatMessage = {
  /** Server uuid, or a local id until `message_start` arrives. */
  id: string;
  role: 'user' | 'assistant';
  status: 'pending' | 'complete' | 'error' | 'cancelled';
  blocks: ChatBlock[];
};

/** One parsed event of a turn. */
export type StreamEvent =
  | { event: 'message_start'; data: { conversation_uuid: string; message_uuid: string; user_message_uuid?: string } }
  | { event: 'text_delta'; data: { text: string } }
  | { event: 'tool_start'; data: { id: string; name: string } }
  | { event: 'tool_result'; data: { id: string; name: string; ok: boolean; code?: string } }
  | { event: 'confirm'; data: AIActionCard }
  | { event: 'action'; data: AIActionOutcome }
  | { event: 'quota_exceeded'; data: { limit: number; used: number } }
  | { event: 'error'; data: { code: string; message?: string } }
  | { event: 'message_done'; data: { message_uuid: string; status: string; stop_reason?: string } }
  | { event: 'title'; data: { conversation_uuid: string; title: string } };

function toolStatus(s: string | undefined): ToolStatus {
  return s === 'done' || s === 'error' || s === 'pending' ? s : 'running';
}

/** Maps a stored message (rendered `ui` blocks) to the chat model. */
export function fromServerMessage(message: AIMessage): ChatMessage {
  const blocks: ChatBlock[] = [];
  for (const b of message.ui) {
    if (b.type === 'text' && b.text) blocks.push({ type: 'text', text: b.text });
    else if (b.type === 'tool') blocks.push({ type: 'tool', id: b.id ?? '', name: b.name ?? '', status: toolStatus(b.status) });
    else if (b.type === 'confirm' && b.card) blocks.push({ type: 'confirm', card: b.card });
    else if (b.type === 'action' && b.action) blocks.push({ type: 'action', action: b.action });
    else if (b.type === 'error') blocks.push({ type: 'error', code: b.code ?? '', message: b.message });
  }
  return { id: message.uuid, role: message.role, status: message.status, blocks };
}

function patch(messages: ChatMessage[], id: string, update: (m: ChatMessage) => ChatMessage): ChatMessage[] {
  return messages.map((m) => (m.id === id ? update(m) : m));
}

/** Marks every card of an action with a new status. */
export function setCardStatus(messages: ChatMessage[], actionUuid: string, status: AIActionCard['status']): ChatMessage[] {
  return messages.map((m) =>
    m.blocks.some((b) => b.type === 'confirm' && b.card.action_uuid === actionUuid)
      ? {
          ...m,
          blocks: m.blocks.map((b) =>
            b.type === 'confirm' && b.card.action_uuid === actionUuid ? { ...b, card: { ...b.card, status } } : b,
          ),
        }
      : m,
  );
}

/**
 * Applies one stream event to the assistant message being streamed (`id`).
 * Returns the new list and the message id (message_start swaps the local id
 * for the server uuid). Consecutive `text_delta` parts join one text block.
 */
export function applyStreamEvent(
  messages: ChatMessage[],
  id: string,
  event: StreamEvent,
): { messages: ChatMessage[]; id: string } {
  switch (event.event) {
    case 'message_start': {
      const next = event.data.message_uuid;
      if (!next || next === id) return { messages, id };
      return { messages: patch(messages, id, (m) => ({ ...m, id: next })), id: next };
    }
    case 'text_delta':
      return {
        id,
        messages: patch(messages, id, (m) => {
          const last = m.blocks[m.blocks.length - 1];
          if (last?.type === 'text') {
            return { ...m, blocks: [...m.blocks.slice(0, -1), { type: 'text', text: last.text + event.data.text }] };
          }
          return { ...m, blocks: [...m.blocks, { type: 'text', text: event.data.text }] };
        }),
      };
    case 'tool_start':
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          blocks: [...m.blocks, { type: 'tool', id: event.data.id, name: event.data.name, status: 'running' }],
        })),
      };
    case 'tool_result':
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          blocks: m.blocks.map((b) =>
            b.type === 'tool' && b.id === event.data.id ? { ...b, status: event.data.ok ? 'done' : 'error' } : b,
          ),
        })),
      };
    case 'confirm':
      return {
        id,
        messages: patch(messages, id, (m) => ({ ...m, blocks: [...m.blocks, { type: 'confirm', card: event.data }] })),
      };
    case 'action': {
      // The card the outcome answers is closed wherever it is.
      const resolved = setCardStatus(messages, event.data.action_uuid, event.data.status);
      return {
        id,
        messages: patch(resolved, id, (m) => ({ ...m, blocks: [...m.blocks, { type: 'action', action: event.data }] })),
      };
    }
    case 'error':
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          status: 'error',
          blocks: [...m.blocks, { type: 'error', code: event.data.code, message: event.data.message }],
        })),
      };
    case 'message_done':
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          status: event.data.status === 'error' ? 'error' : event.data.status === 'cancelled' ? 'cancelled' : 'complete',
        })),
      };
    default:
      return { messages, id };
  }
}

/** Text of a message's text blocks. */
export function messageText(message: ChatMessage): string {
  return message.blocks
    .filter((b): b is { type: 'text'; text: string } => b.type === 'text')
    .map((b) => b.text)
    .join('');
}

/** Remaining quota in percent, null when the quota is unlimited. */
export function quotaRemainingPercent(quota: { limit: number; used: number }): number | null {
  if (!quota.limit || quota.limit <= 0) return null;
  const left = Math.max(0, quota.limit - quota.used);
  return Math.round((left / quota.limit) * 100);
}

/** Whether the monthly quota is used up (status or a quota_exceeded event). */
export function quotaUsedUp(quota: AIStatus['quota'] | undefined): boolean {
  return !!quota && quota.limit > 0 && (quota.remaining ?? 1) <= 0;
}

/** Seconds until `iso` (never negative). */
export function secondsUntil(iso: string, now: number): number {
  const t = Date.parse(iso);
  return Number.isNaN(t) ? 0 : Math.max(0, Math.ceil((t - now) / 1000));
}

/** "m:ss" (or "h:mm:ss") countdown. */
export function formatCountdown(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`;
}

/** Initial values of a card's editable fields. */
export function initialEdits(preview: ActionPreview): Record<string, string> {
  return Object.fromEntries((preview.edit ?? []).map((f) => [f.key, f.value ?? '']));
}

/** True when a required editable field is empty. */
export function missingRequired(preview: ActionPreview, values: Record<string, string>): boolean {
  return (preview.edit ?? []).some((f) => f.required && !(values[f.key] ?? '').trim());
}

/**
 * Edits to send with the confirmation: every editable field (numbers as
 * numbers), undefined when the card has none.
 */
export function editsToSend(preview: ActionPreview, values: Record<string, string>): Record<string, unknown> | undefined {
  const fields = preview.edit ?? [];
  if (!fields.length) return undefined;
  const edits: Record<string, unknown> = {};
  for (const f of fields) {
    const raw = (values[f.key] ?? '').trim();
    edits[f.key] = f.type === 'number' && raw !== '' ? Number(raw) : raw;
  }
  return edits;
}

/** Mobile screen of a record a confirmed action created (null: none on mobile). */
export function recordPath(space: string, kind: string, uuid: string): string | null {
  switch (kind) {
    case 'lead':
      return `/${space}/menu/leads/${uuid}`;
    case 'order':
      return `/${space}/menu/orders/${uuid}`;
    case 'task':
      return space === 'admin' ? `/admin/menu/tasks/${uuid}` : null;
    default:
      return null;
  }
}
