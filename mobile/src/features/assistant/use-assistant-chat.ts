import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useRef, useState } from 'react';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';

import {
  type AIActionCard,
  type AIConversation,
  applyStreamEvent,
  type ChatMessage,
  fromServerMessage,
  type LegalText,
} from './chat-state';
import { AssistantRequestError, streamTurn } from './stream';

export const aiKeys = {
  status: ['ai', 'status'] as const,
  conversations: ['ai', 'conversations'] as const,
  pending: ['ai', 'pending-actions'] as const,
};

export function useAssistantStatus() {
  return useQuery({ queryKey: aiKeys.status, queryFn: () => unwrap(api.GET('/v1/ai/status')), retry: false });
}

export function useConversations(enabled: boolean) {
  return useQuery({
    queryKey: aiKeys.conversations,
    queryFn: async () =>
      (await unwrap(api.GET('/v1/ai/conversations', { params: { query: { limit: 50, sort: '-updated_at' } } }))).items,
    enabled,
  });
}

/** Accepts the AI guidelines text the status (or a 428 answer) carried. */
export async function acceptConsent(text: LegalText) {
  await unwrap(
    api.POST('/v1/consents', { body: { kind: text.kind, locale: text.locale, version: text.version, accepted: true } }),
  );
}

type Replay =
  | { kind: 'message'; content: string }
  | { kind: 'confirm'; action: string; edits: Record<string, unknown> | undefined }
  | { kind: 'cancel'; action: string };

let seq = 0;
const localId = (prefix: string) => `${prefix}-${Date.now()}-${++seq}`;

/**
 * One conversation of the panel assistant: loads its messages, streams new
 * turns (one at a time; `stop` aborts and the server stores it as
 * cancelled), confirms / cancels proposed actions and handles the consent
 * and quota answers. `initial` is a conversation uuid or null for a new one
 * (created on the first message; `onCreated` gets its uuid).
 */
export function useAssistantChat(initial: string | null, onCreated: (uuid: string) => void) {
  const qc = useQueryClient();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(!!initial);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [streaming, setStreaming] = useState(false);
  const [quotaHit, setQuotaHit] = useState(false);
  const [consent, setConsent] = useState<LegalText | null>(null);
  const [lastReplay, setLastReplay] = useState<Replay | null>(null);
  const [title, setTitle] = useState<string | null>(null);
  const conversation = useRef<string | null>(initial);
  const draft = useRef('');
  const abort = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    const uuid = conversation.current;
    if (!uuid) return;
    setLoading(true);
    setLoadError(null);
    try {
      const detail = await unwrap(api.GET('/v1/ai/conversations/{uuid}', { params: { path: { uuid } } }));
      if (conversation.current === uuid) {
        setMessages(detail.messages.map(fromServerMessage));
        setTitle(detail.title || null);
      }
    } catch (e) {
      setLoadError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    return () => abort.current?.abort();
  }, [load]);

  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: aiKeys.conversations });
    void qc.invalidateQueries({ queryKey: aiKeys.status });
  }, [qc]);

  /** Runs one streamed request; false when it was refused before streaming. */
  const run = useCallback(
    async (path: string, body: unknown, replay: Replay, userText?: string): Promise<boolean> => {
      const controller = new AbortController();
      abort.current = controller;
      const userId = userText ? localId('user') : null;
      const draftId = localId('assistant');
      draft.current = draftId;
      setLastReplay(replay);
      setStreaming(true);
      setMessages((list) => [
        ...list,
        ...(userId && userText
          ? [{ id: userId, role: 'user' as const, status: 'complete' as const, blocks: [{ type: 'text' as const, text: userText }] }]
          : []),
        { id: draftId, role: 'assistant', status: 'pending', blocks: [] },
      ]);
      try {
        await streamTurn(path, body, controller.signal, (event) => {
          if (event.event === 'quota_exceeded') {
            setQuotaHit(true);
            return;
          }
          if (event.event === 'title') {
            setTitle(event.data.title);
            qc.setQueryData<AIConversation[]>(aiKeys.conversations, (list) =>
              list?.map((c) => (c.uuid === event.data.conversation_uuid ? { ...c, title: event.data.title } : c)),
            );
            return;
          }
          if (event.event === 'message_start' && event.data.user_message_uuid && userId) {
            const serverUser = event.data.user_message_uuid;
            setMessages((list) => list.map((m) => (m.id === userId ? { ...m, id: serverUser } : m)));
          }
          const id = draft.current;
          if (event.event === 'message_start' && event.data.message_uuid) draft.current = event.data.message_uuid;
          setMessages((list) => applyStreamEvent(list, id, event).messages);
        });
        // A stream that ended without message_done (connection dropped).
        const finished = draft.current;
        setMessages((list) => list.map((m) => (m.id === finished && m.status === 'pending' ? { ...m, status: 'complete' } : m)));
        return true;
      } catch (error) {
        const current = draft.current;
        if (controller.signal.aborted) {
          setMessages((list) => list.map((m) => (m.id === current ? { ...m, status: 'cancelled' } : m)));
          return true;
        }
        if (error instanceof AssistantRequestError && (error.code === 'AI_CONSENT_REQUIRED' || error.code === 'AI_QUOTA_EXCEEDED')) {
          setMessages((list) => list.filter((m) => m.id !== draftId && m.id !== userId));
          if (error.code === 'AI_QUOTA_EXCEEDED') setQuotaHit(true);
          else if (error.consent) setConsent(error.consent);
          return false;
        }
        const code = isApiError(error) && error.status > 0 ? error.code : 'network';
        const message = isApiError(error) && error.status > 0 ? error.message : undefined;
        setMessages((list) =>
          list.map((m) => (m.id === current ? { ...m, status: 'error', blocks: [...m.blocks, { type: 'error', code, message }] } : m)),
        );
        return true;
      } finally {
        if (abort.current === controller) abort.current = null;
        setStreaming(false);
        refresh();
      }
    },
    [qc, refresh],
  );

  const send = useCallback(
    async (content: string): Promise<boolean> => {
      const text = content.trim();
      if (!text || streaming) return false;
      if (!conversation.current) {
        try {
          const created = await unwrap(api.POST('/v1/ai/conversations', { body: {} }));
          conversation.current = created.uuid;
          qc.setQueryData<AIConversation[]>(aiKeys.conversations, (list) => [created, ...(list ?? [])]);
          onCreated(created.uuid);
        } catch (e) {
          if (isApiError(e, 'AI_CONSENT_REQUIRED')) void qc.invalidateQueries({ queryKey: aiKeys.status });
          return false;
        }
      }
      return run(`/v1/ai/conversations/${conversation.current}/messages`, { content: text }, { kind: 'message', content: text }, text);
    },
    [onCreated, qc, run, streaming],
  );

  const confirm = useCallback(
    (card: AIActionCard, edits?: Record<string, unknown>) =>
      run(`/v1/ai/actions/${card.action_uuid}/confirm`, edits ? { edits } : {}, {
        kind: 'confirm',
        action: card.action_uuid,
        edits,
      }),
    [run],
  );

  const cancel = useCallback(
    (card: AIActionCard) =>
      run(`/v1/ai/actions/${card.action_uuid}/cancel`, {}, { kind: 'cancel', action: card.action_uuid }),
    [run],
  );

  /** Repeats the last request after an error (no new user bubble). */
  const retry = useCallback(() => {
    const replay = lastReplay;
    const uuid = conversation.current;
    if (!replay || streaming) return;
    setMessages((list) => list.filter((m) => !(m.role === 'assistant' && m.status === 'error')));
    if (replay.kind === 'message' && uuid) void run(`/v1/ai/conversations/${uuid}/messages`, { content: replay.content }, replay);
    else if (replay.kind === 'confirm')
      void run(`/v1/ai/actions/${replay.action}/confirm`, replay.edits ? { edits: replay.edits } : {}, replay);
    else if (replay.kind === 'cancel') void run(`/v1/ai/actions/${replay.action}/cancel`, {}, replay);
  }, [lastReplay, run, streaming]);

  const stop = useCallback(() => abort.current?.abort(), []);

  return {
    messages,
    loading,
    loadError,
    reload: load,
    streaming,
    quotaHit,
    consent,
    clearConsent: () => setConsent(null),
    title,
    canRetry: lastReplay !== null,
    send,
    confirm,
    cancel,
    retry,
    stop,
  };
}

