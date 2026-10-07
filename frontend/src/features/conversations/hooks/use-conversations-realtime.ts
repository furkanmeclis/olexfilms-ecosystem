"use client";

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { realtimeConfig } from "@/config/realtime";
import { conversationsKeys } from "@/features/conversations/hooks/query-keys";
import {
  upsertMessage,
  type MessagePages,
} from "@/features/conversations/lib/conversations";
import type {
  Conversation,
  ConversationMessage,
} from "@/features/conversations/services/conversations.service";
import { useChannel } from "@/hooks/use-realtime";
import { RealtimeChannels } from "@/lib/realtime/channels";
import { RealtimeEvents } from "@/lib/realtime/events";
import type { RealtimeMessage } from "@/lib/realtime/types";

const MESSAGE_EVENTS = new Set<string>([
  RealtimeEvents.ConversationMessageCreated,
  RealtimeEvents.ConversationMessageUpdated,
]);

function findMessage(data: MessagePages | undefined, uuid: string) {
  for (const page of data?.pages ?? []) {
    const hit = page.items.find((item) => item.uuid === uuid);
    if (hit) return hit;
  }
  return undefined;
}

/**
 * Applies a `system.conversations` publication to the query cache: message
 * events are merged into the open timeline, conversation events replace
 * the detail; lists and the unread badge are refetched.
 */
export function useApplyConversationEvent() {
  const queryClient = useQueryClient();
  return useCallback(
    (event: RealtimeMessage) => {
      const data = event.data as {
        conversation_uuid?: string;
        message?: Partial<ConversationMessage>;
        conversation?: Conversation;
      };
      if (MESSAGE_EVENTS.has(event.type)) {
        const message = data.message;
        const conversationUuid =
          data.conversation_uuid ?? message?.conversation_uuid;
        if (!message?.uuid || !conversationUuid) return;
        const key = conversationsKeys.messages(conversationUuid);
        queryClient.setQueryData<MessagePages>(key, (pages) => {
          if (!pages) return pages;
          const existing = findMessage(pages, message.uuid!);
          const merged = {
            ...existing,
            ...message,
            conversation_uuid: conversationUuid,
            sender_user: existing?.sender_user ?? null,
          } as ConversationMessage;
          return upsertMessage(pages, merged);
        });
      } else if (event.type === RealtimeEvents.ConversationUpdated) {
        if (data.conversation?.uuid) {
          queryClient.setQueryData(
            conversationsKeys.detail(data.conversation.uuid),
            data.conversation,
          );
        }
      } else {
        return;
      }
      void queryClient.invalidateQueries({
        queryKey: conversationsKeys.lists(),
      });
      void queryClient.invalidateQueries({
        queryKey: conversationsKeys.unread(),
      });
    },
    [queryClient],
  );
}

/** Live inbox updates over Centrifugo (platform admins only, S2). */
export function useConversationsRealtime(enabled = true) {
  const apply = useApplyConversationEvent();
  useChannel(RealtimeChannels.systemConversations(), apply, {
    enabled: enabled && realtimeConfig.enabled,
  });
}
