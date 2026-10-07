import type { InfiniteData } from "@tanstack/react-query";

import type {
  Conversation,
  ConversationAIMode,
  ConversationIdentityKind,
  ConversationMessage,
  ConversationMessagePage,
  ConversationStatus,
} from "@/features/conversations/services/conversations.service";

export const CONVERSATION_STATUSES: ConversationStatus[] = [
  "open",
  "pending",
  "closed",
];
export const CONVERSATION_AI_MODES: ConversationAIMode[] = [
  "auto",
  "paused",
  "off",
];
export const CONVERSATION_IDENTITY_KINDS: ConversationIdentityKind[] = [
  "panel_user",
  "customer",
  "visitor",
  "unknown",
];

export const STATUS_TONE: Record<
  ConversationStatus,
  "success" | "warning" | "default"
> = {
  open: "success",
  pending: "warning",
  closed: "default",
};

export const AI_MODE_TONE: Record<
  ConversationAIMode,
  "success" | "warning" | "danger"
> = {
  auto: "success",
  paused: "warning",
  off: "danger",
};

/** WhatsApp attachment limit of the reply endpoint (TEC-398). */
export const MAX_ATTACHMENT_BYTES = 16 * 1024 * 1024;
export const ATTACHMENT_MIME_TYPES = [
  "image/jpeg",
  "image/png",
  "image/webp",
  "application/pdf",
] as const;
export const ATTACHMENT_ACCEPT = ATTACHMENT_MIME_TYPES.join(",");

/** Staff reply pauses the AI for this long (backend rule). */
export const STAFF_REPLY_AI_PAUSE_MINUTES = 30;

export type AttachmentError = "too_large" | "type";

export function validateAttachment(file: File): AttachmentError | null {
  if (!(ATTACHMENT_MIME_TYPES as readonly string[]).includes(file.type)) {
    return "type";
  }
  if (file.size > MAX_ATTACHMENT_BYTES) return "too_large";
  return null;
}

export function conversationTitle(conversation: Conversation) {
  return (
    conversation.contact_name?.trim() ||
    conversation.identity_user?.name ||
    conversation.contact_e164
  );
}

export function conversationInitials(conversation: Conversation) {
  const title = conversationTitle(conversation).replace(/^\+/, "");
  const parts = title.split(/\s+/).filter(Boolean);
  const letters = parts
    .slice(0, 2)
    .map((part) => part[0])
    .join("");
  return (letters || "?").toUpperCase();
}

export function messageMedia(message: ConversationMessage) {
  const media = (message.media ?? {}) as Record<string, unknown>;
  const str = (key: string) =>
    typeof media[key] === "string" ? (media[key] as string) : undefined;
  const mime = message.media_mime || str("mime_type");
  if (!mime && !message.has_stored_media && !str("type")) return null;
  return {
    mime,
    fileName: str("file_name"),
    caption: str("caption"),
    isImage: Boolean(mime?.startsWith("image/")) || str("type") === "image",
  };
}

export type MessagePages = InfiniteData<ConversationMessagePage, unknown>;

/**
 * Inserts or replaces a message in the newest-first timeline cache. A
 * message already present (same uuid) is replaced in place; a new one goes
 * to the front of the first page.
 */
export function upsertMessage(
  data: MessagePages | undefined,
  message: ConversationMessage,
  replaceUuid?: string,
): MessagePages | undefined {
  if (!data) return data;
  let found = false;
  const pages = data.pages.map((page) => ({
    ...page,
    items: page.items.flatMap((item) => {
      if (item.uuid === message.uuid || item.uuid === replaceUuid) {
        if (found) return [];
        found = true;
        return [message];
      }
      return [item];
    }),
  }));
  if (!found) {
    const [first, ...rest] = pages;
    const head = first ?? { items: [], next_cursor: null };
    return {
      ...data,
      pages: [{ ...head, items: [message, ...head.items] }, ...rest],
    };
  }
  return { ...data, pages };
}

export function removeMessage(
  data: MessagePages | undefined,
  uuid: string,
): MessagePages | undefined {
  if (!data) return data;
  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      items: page.items.filter((item) => item.uuid !== uuid),
    })),
  };
}

/** Oldest-first flat list for rendering. */
export function flattenMessages(data: MessagePages | undefined) {
  if (!data) return [];
  const seen = new Set<string>();
  const out: ConversationMessage[] = [];
  for (const page of data.pages) {
    for (const item of page.items) {
      if (seen.has(item.uuid)) continue;
      seen.add(item.uuid);
      out.push(item);
    }
  }
  return out.reverse();
}
