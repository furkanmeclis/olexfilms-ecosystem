"use client";

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { conversationsKeys } from "@/features/conversations/hooks/query-keys";
import {
  removeMessage,
  upsertMessage,
  type MessagePages,
} from "@/features/conversations/lib/conversations";
import {
  conversationsService,
  type Conversation,
  type ConversationListParams,
  type ConversationMessage,
  type ConversationPatchInput,
} from "@/features/conversations/services/conversations.service";
import { usersService } from "@/features/users/services/users.service";
import { userFullName } from "@/features/users/lib/user-display";

export function useConversationsMeta(enabled = true) {
  return useQuery({
    queryKey: conversationsKeys.meta(),
    queryFn: () => conversationsService.meta(),
    enabled,
    staleTime: 5 * 60_000,
  });
}

export function useConversationsList(
  params: ConversationListParams,
  enabled = true,
) {
  return useQuery({
    queryKey: conversationsKeys.list(params),
    queryFn: () => conversationsService.list(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useConversation(uuid: string | null) {
  return useQuery({
    queryKey: conversationsKeys.detail(uuid ?? ""),
    queryFn: () => conversationsService.get(uuid!),
    enabled: Boolean(uuid),
  });
}

/** Timeline, newest first; older pages through the `before` cursor. */
export function useConversationMessages(uuid: string | null) {
  return useInfiniteQuery({
    queryKey: conversationsKeys.messages(uuid ?? ""),
    queryFn: ({ pageParam }) => conversationsService.messages(uuid!, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    enabled: Boolean(uuid),
  });
}

export function useConversationAIRuns(uuid: string | null, enabled = true) {
  return useQuery({
    queryKey: conversationsKeys.aiRuns(uuid ?? ""),
    queryFn: () => conversationsService.aiRuns(uuid!),
    enabled: Boolean(uuid) && enabled,
  });
}

/** Number of conversations with unread messages (menu badge). */
export function useUnreadConversations(enabled = true) {
  return useQuery({
    queryKey: conversationsKeys.unread(),
    queryFn: () =>
      conversationsService.list({ limit: 1, offset: 0, unread: "true" }),
    enabled,
    refetchInterval: 60_000,
    select: (page) => page.total,
  });
}

/** Platform admins: the only valid assignees (S2). */
export function usePlatformAdmins(enabled = true) {
  return useQuery({
    queryKey: conversationsKeys.admins(),
    queryFn: () =>
      usersService.list({
        limit: 100,
        offset: 0,
        role: "super_admin",
        status: "active",
      }),
    enabled,
    staleTime: 5 * 60_000,
    select: (page) =>
      page.items.map((user) => ({
        value: user.uuid,
        label: userFullName(user),
      })),
  });
}

function useSetConversation() {
  const queryClient = useQueryClient();
  return (conversation: Conversation) => {
    queryClient.setQueryData(
      conversationsKeys.detail(conversation.uuid),
      conversation,
    );
    void queryClient.invalidateQueries({
      queryKey: conversationsKeys.lists(),
    });
    void queryClient.invalidateQueries({
      queryKey: conversationsKeys.unread(),
    });
  };
}

export function usePatchConversation(uuid: string) {
  const setConversation = useSetConversation();
  return useMutation({
    mutationFn: (body: ConversationPatchInput) =>
      conversationsService.patch(uuid, body),
    onSuccess: setConversation,
  });
}

/** Close / reopen from a list row. */
export function useSetConversationStatus() {
  const setConversation = useSetConversation();
  return useMutation({
    mutationFn: ({
      uuid,
      status,
    }: {
      uuid: string;
      status: Conversation["status"];
    }) => conversationsService.patch(uuid, { status }),
    onSuccess: setConversation,
  });
}

export function useMarkConversationRead() {
  const setConversation = useSetConversation();
  return useMutation({
    mutationFn: (uuid: string) => conversationsService.markRead(uuid),
    onSuccess: setConversation,
  });
}

export type ReplyVariables = {
  body: string;
  file?: File | null;
  sender?: { uuid: string; name: string } | null;
};

let optimisticSeq = 0;

/**
 * Staff reply with an optimistic `staff` bubble: the temporary message is
 * replaced by the queued one on success and removed on failure.
 */
export function useReplyConversation(uuid: string) {
  const queryClient = useQueryClient();
  const setConversation = useSetConversation();
  const key = conversationsKeys.messages(uuid);

  return useMutation({
    mutationFn: ({ body, file }: ReplyVariables) =>
      conversationsService.reply(uuid, body, file),
    onMutate: async ({ body, file, sender }) => {
      await queryClient.cancelQueries({ queryKey: key });
      optimisticSeq += 1;
      const tempUuid = `optimistic-${Date.now()}-${optimisticSeq}`;
      const temp: ConversationMessage = {
        uuid: tempUuid,
        conversation_uuid: uuid,
        direction: "out",
        sender_type: "staff",
        status: "queued",
        body: body || null,
        has_stored_media: false,
        media: file
          ? { type: file.type.startsWith("image/") ? "image" : "document" }
          : undefined,
        media_mime: file?.type,
        media_size: file?.size,
        send_attempts: 0,
        created_at: new Date().toISOString(),
        sender_user: sender ?? null,
      };
      queryClient.setQueryData<MessagePages>(key, (data) =>
        upsertMessage(
          data ?? {
            pages: [{ items: [], next_cursor: null }],
            pageParams: [null],
          },
          temp,
        ),
      );
      return { tempUuid };
    },
    onSuccess: (result, _vars, context) => {
      queryClient.setQueryData<MessagePages>(key, (data) =>
        upsertMessage(data, result.message, context?.tempUuid),
      );
      setConversation(result.conversation);
    },
    onError: (_error, _vars, context) => {
      if (!context) return;
      queryClient.setQueryData<MessagePages>(key, (data) =>
        removeMessage(data, context.tempUuid),
      );
    },
  });
}

export function useStartConversation() {
  const setConversation = useSetConversation();
  return useMutation({
    mutationFn: ({
      userUuid,
      body,
      file,
    }: {
      userUuid: string;
      body: string;
      file?: File | null;
    }) => conversationsService.start(userUuid, body, file),
    onSuccess: (result) => setConversation(result.conversation),
  });
}
