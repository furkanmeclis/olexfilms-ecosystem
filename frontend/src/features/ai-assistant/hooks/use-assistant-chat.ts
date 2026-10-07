"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { applyStreamEvent, fromServerMessage } from "../lib/chat-state";
import { readEventStream } from "../lib/sse";
import {
  AssistantRequestError,
  type AIActionCard,
  type AIConversation,
  type AssistantTransport,
  type ChatMessage,
  type LegalText,
} from "../lib/types";

let localSeq = 0;
function localId(prefix: string) {
  localSeq += 1;
  return `${prefix}-${Date.now()}-${localSeq}`;
}

type Replay =
  | { kind: "message"; content: string }
  | {
      kind: "confirm";
      action: string;
      edits: Record<string, unknown> | undefined;
    }
  | { kind: "cancel"; action: string };

export function assistantQueryKey(scope: string) {
  return ["ai-assistant", scope] as const;
}

/**
 * State of one assistant chat: status (consent, quota), the conversation
 * list, the open conversation and the streamed turn. One turn streams at a
 * time; `stop` aborts the request (the server stores it as cancelled).
 */
export function useAssistantChat(transport: AssistantTransport, scope: string) {
  const qc = useQueryClient();
  const key = useMemo(() => assistantQueryKey(scope), [scope]);

  const status = useQuery({
    queryKey: [...key, "status"],
    queryFn: () => transport.status(),
    retry: false,
  });
  const conversations = useQuery({
    queryKey: [...key, "conversations"],
    queryFn: () => transport.listConversations(),
    enabled: Boolean(status.data?.enabled && status.data.allowed),
    retry: false,
  });

  const [activeId, setActiveId] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loadingConversation, setLoadingConversation] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [quotaHit, setQuotaHit] = useState(false);
  const [consentText, setConsentText] = useState<LegalText | null>(null);
  const [consentDismissed, setConsentDismissed] = useState(false);
  const [lastReplay, setLastReplay] = useState<Replay | null>(null);

  const abortRef = useRef<AbortController | null>(null);
  const draftRef = useRef<string>("");
  const activeRef = useRef<string | null>(null);
  const select = useCallback((uuid: string | null) => {
    activeRef.current = uuid;
    setActiveId(uuid);
  }, []);

  // Status (or a 428 answer) asks for the guidelines: the dialog is open
  // until the user accepts or closes it.
  const statusConsent = status.data?.consent_required
    ? (status.data.consent ?? null)
    : null;
  const pendingConsent = consentText ?? statusConsent;
  const consentOpen = pendingConsent !== null && !consentDismissed;
  const setConsentOpen = useCallback((open: boolean) => {
    setConsentDismissed(!open);
  }, []);

  useEffect(() => () => abortRef.current?.abort(), []);

  const quota = status.data?.quota;
  const quotaExceeded =
    quotaHit ||
    Boolean(quota && quota.limit > 0 && (quota.remaining ?? 1) <= 0);
  const consentRequired =
    Boolean(status.data?.consent_required) || consentText !== null;

  const patchConversation = useCallback(
    (uuid: string, update: Partial<AIConversation>) => {
      qc.setQueryData<AIConversation[]>([...key, "conversations"], (list) =>
        list?.map((c) => (c.uuid === uuid ? { ...c, ...update } : c)),
      );
    },
    [qc, key],
  );

  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: [...key, "conversations"] });
    void qc.invalidateQueries({ queryKey: [...key, "status"] });
  }, [qc, key]);

  /**
   * Runs one streamed request. `userText` adds the user's bubble first.
   * Returns false when the request was refused before streaming (the
   * composer then keeps the text).
   */
  const run = useCallback(
    async (
      start: (signal: AbortSignal) => Promise<ReadableStream<Uint8Array>>,
      replay: Replay,
      userText?: string,
    ): Promise<boolean> => {
      const controller = new AbortController();
      abortRef.current = controller;
      const userId = userText ? localId("user") : null;
      const draftId = localId("assistant");
      draftRef.current = draftId;
      setLastReplay(replay);
      setStreaming(true);
      setMessages((list) => [
        ...list,
        ...(userId
          ? [
              {
                id: userId,
                role: "user" as const,
                status: "complete" as const,
                blocks: [{ type: "text" as const, text: userText! }],
              },
            ]
          : []),
        { id: draftId, role: "assistant", status: "pending", blocks: [] },
      ]);

      try {
        const stream = await start(controller.signal);
        await readEventStream(stream, (event) => {
          if (event.event === "quota_exceeded") {
            setQuotaHit(true);
            return;
          }
          if (event.event === "title") {
            patchConversation(event.data.conversation_uuid, {
              title: event.data.title,
            });
            return;
          }
          if (
            event.event === "message_start" &&
            event.data.user_message_uuid &&
            userId
          ) {
            const serverUser = event.data.user_message_uuid;
            setMessages((list) =>
              list.map((m) => (m.id === userId ? { ...m, id: serverUser } : m)),
            );
          }
          // State updaters stay pure (Strict Mode runs them twice): the
          // id swap of message_start is decided here, not inside them.
          const id = draftRef.current;
          if (event.event === "message_start" && event.data.message_uuid) {
            draftRef.current = event.data.message_uuid;
          }
          setMessages((list) => applyStreamEvent(list, id, event).messages);
        });
        // A stream that ended without message_done (connection dropped).
        const finished = draftRef.current;
        setMessages((list) =>
          list.map((m) =>
            m.id === finished && m.status === "pending"
              ? { ...m, status: "complete" }
              : m,
          ),
        );
        return true;
      } catch (error) {
        const current = draftRef.current;
        if (controller.signal.aborted) {
          setMessages((list) =>
            list.map((m) =>
              m.id === current ? { ...m, status: "cancelled" } : m,
            ),
          );
          return true;
        }
        if (error instanceof AssistantRequestError) {
          if (error.code === "AI_CONSENT_REQUIRED") {
            setMessages((list) =>
              list.filter((m) => m.id !== draftId && m.id !== userId),
            );
            if (error.consent) setConsentText(error.consent);
            setConsentDismissed(false);
            return false;
          }
          if (error.code === "AI_QUOTA_EXCEEDED") {
            setQuotaHit(true);
            setMessages((list) =>
              list.filter((m) => m.id !== draftId && m.id !== userId),
            );
            return false;
          }
        }
        const message =
          error instanceof Error && error.message ? error.message : undefined;
        const code =
          error instanceof AssistantRequestError ? error.code : "network";
        setMessages((list) =>
          list.map((m) =>
            m.id === current
              ? {
                  ...m,
                  status: "error",
                  blocks: [...m.blocks, { type: "error", code, message }],
                }
              : m,
          ),
        );
        return true;
      } finally {
        if (abortRef.current === controller) abortRef.current = null;
        setStreaming(false);
        refresh();
      }
    },
    [patchConversation, refresh],
  );

  const send = useCallback(
    async (content: string): Promise<boolean> => {
      const text = content.trim();
      if (!text || streaming) return false;
      let conversation = activeRef.current;
      if (!conversation) {
        try {
          const created = await transport.createConversation();
          conversation = created.uuid;
          qc.setQueryData<AIConversation[]>(
            [...key, "conversations"],
            (list) => [created, ...(list ?? [])],
          );
          select(created.uuid);
        } catch {
          return false;
        }
      }
      const target = conversation;
      return run(
        (signal) => transport.sendMessage(target, text, signal),
        { kind: "message", content: text },
        text,
      );
    },
    [run, streaming, transport, qc, key, select],
  );

  const confirm = useCallback(
    (card: AIActionCard, edits?: Record<string, unknown>) =>
      run(
        (signal) => transport.confirmAction(card.action_uuid, edits, signal),
        { kind: "confirm", action: card.action_uuid, edits },
      ),
    [run, transport],
  );

  const cancel = useCallback(
    (card: AIActionCard) =>
      run((signal) => transport.cancelAction(card.action_uuid, signal), {
        kind: "cancel",
        action: card.action_uuid,
      }),
    [run, transport],
  );

  /** Repeats the last request after an error (no new user bubble). */
  const retry = useCallback(() => {
    const replay = lastReplay;
    const conversation = activeRef.current;
    if (!replay || streaming) return;
    setMessages((list) =>
      list.filter((m) => !(m.role === "assistant" && m.status === "error")),
    );
    if (replay.kind === "message" && conversation) {
      void run(
        (signal) => transport.sendMessage(conversation, replay.content, signal),
        replay,
      );
    } else if (replay.kind === "confirm") {
      void run(
        (signal) =>
          transport.confirmAction(replay.action, replay.edits, signal),
        replay,
      );
    } else if (replay.kind === "cancel") {
      void run(
        (signal) => transport.cancelAction(replay.action, signal),
        replay,
      );
    }
  }, [lastReplay, run, streaming, transport]);

  const stop = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const open = useCallback(
    async (uuid: string | null) => {
      if (streaming) return;
      select(uuid);
      setLastReplay(null);
      if (!uuid) {
        setMessages([]);
        return;
      }
      setLoadingConversation(true);
      try {
        const detail = await transport.getConversation(uuid);
        if (activeRef.current === uuid) {
          setMessages(detail.messages.map(fromServerMessage));
        }
      } catch {
        setMessages([]);
      } finally {
        setLoadingConversation(false);
      }
    },
    [streaming, transport, select],
  );

  const rename = useCallback(
    async (uuid: string, title: string) => {
      const updated = await transport.renameConversation(uuid, title);
      patchConversation(uuid, { title: updated.title });
    },
    [patchConversation, transport],
  );

  const remove = useCallback(
    async (uuid: string) => {
      await transport.deleteConversation(uuid);
      qc.setQueryData<AIConversation[]>([...key, "conversations"], (list) =>
        list?.filter((c) => c.uuid !== uuid),
      );
      if (activeRef.current === uuid) {
        select(null);
        setMessages([]);
      }
    },
    [qc, transport, key, select],
  );

  const acceptConsent = useCallback(async () => {
    if (!pendingConsent) return;
    await transport.acceptConsent(pendingConsent);
    setConsentText(null);
    await qc.invalidateQueries({ queryKey: [...key, "status"] });
  }, [pendingConsent, transport, qc, key]);

  return useMemo(
    () => ({
      status,
      conversations,
      activeId,
      messages,
      loadingConversation,
      streaming,
      quotaExceeded,
      consentRequired,
      consentText: pendingConsent,
      consentOpen,
      setConsentOpen,
      canRetry: lastReplay !== null,
      send,
      stop,
      confirm,
      cancel,
      retry,
      open,
      rename,
      remove,
      acceptConsent,
    }),
    [
      status,
      conversations,
      activeId,
      messages,
      loadingConversation,
      streaming,
      quotaExceeded,
      consentRequired,
      pendingConsent,
      consentOpen,
      setConsentOpen,
      lastReplay,
      send,
      stop,
      confirm,
      cancel,
      retry,
      open,
      rename,
      remove,
      acceptConsent,
    ],
  );
}

export type AssistantChatState = ReturnType<typeof useAssistantChat>;
