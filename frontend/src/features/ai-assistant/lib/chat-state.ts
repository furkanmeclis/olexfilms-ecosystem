import type {
  AIActionCard,
  AIMessage,
  ChatBlock,
  ChatMessage,
  StreamEvent,
} from "./types";

/** Maps a stored message (rendered `ui` blocks) to the chat model. */
export function fromServerMessage(message: AIMessage): ChatMessage {
  const blocks: ChatBlock[] = [];
  for (const b of message.ui) {
    if (b.type === "text" && b.text)
      blocks.push({ type: "text", text: b.text });
    else if (b.type === "tool")
      blocks.push({
        type: "tool",
        id: b.id ?? "",
        name: b.name ?? "",
        status: toolStatus(b.status),
      });
    else if (b.type === "confirm" && b.card)
      blocks.push({ type: "confirm", card: b.card });
    else if (b.type === "action" && b.action)
      blocks.push({ type: "action", action: b.action });
    else if (b.type === "error")
      blocks.push({ type: "error", code: b.code ?? "", message: b.message });
  }
  return {
    id: message.uuid,
    role: message.role,
    status: message.status,
    blocks,
  };
}

function toolStatus(
  s: string | undefined,
): "running" | "done" | "error" | "pending" {
  return s === "done" || s === "error" || s === "pending" ? s : "running";
}

/** Updates one message of the list. */
function patch(
  messages: ChatMessage[],
  id: string,
  update: (m: ChatMessage) => ChatMessage,
): ChatMessage[] {
  return messages.map((m) => (m.id === id ? update(m) : m));
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
    case "message_start": {
      const next = event.data.message_uuid;
      if (!next || next === id) return { messages, id };
      return {
        messages: patch(messages, id, (m) => ({ ...m, id: next })),
        id: next,
      };
    }
    case "text_delta":
      return {
        id,
        messages: patch(messages, id, (m) => {
          const last = m.blocks[m.blocks.length - 1];
          if (last?.type === "text") {
            return {
              ...m,
              blocks: [
                ...m.blocks.slice(0, -1),
                { type: "text", text: last.text + event.data.text },
              ],
            };
          }
          return {
            ...m,
            blocks: [...m.blocks, { type: "text", text: event.data.text }],
          };
        }),
      };
    case "tool_start":
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          blocks: [
            ...m.blocks,
            {
              type: "tool",
              id: event.data.id,
              name: event.data.name,
              status: "running",
            },
          ],
        })),
      };
    case "tool_result":
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          blocks: m.blocks.map((b) =>
            b.type === "tool" && b.id === event.data.id
              ? { ...b, status: event.data.ok ? "done" : "error" }
              : b,
          ),
        })),
      };
    case "confirm":
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          blocks: [...m.blocks, { type: "confirm", card: event.data }],
        })),
      };
    case "action": {
      // The card the outcome answers is closed wherever it is.
      const resolved = setCardStatus(
        messages,
        event.data.action_uuid,
        event.data.status,
      );
      return {
        id,
        messages: patch(resolved, id, (m) => ({
          ...m,
          blocks: [...m.blocks, { type: "action", action: event.data }],
        })),
      };
    }
    case "error":
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          status: "error",
          blocks: [
            ...m.blocks,
            {
              type: "error",
              code: event.data.code,
              message: event.data.message,
            },
          ],
        })),
      };
    case "message_done":
      return {
        id,
        messages: patch(messages, id, (m) => ({
          ...m,
          status:
            event.data.status === "error"
              ? "error"
              : event.data.status === "cancelled"
                ? "cancelled"
                : "complete",
        })),
      };
    default:
      return { messages, id };
  }
}

/** Marks every card of an action with a new status. */
export function setCardStatus(
  messages: ChatMessage[],
  actionUuid: string,
  status: AIActionCard["status"],
): ChatMessage[] {
  return messages.map((m) => {
    if (
      !m.blocks.some(
        (b) => b.type === "confirm" && b.card.action_uuid === actionUuid,
      )
    ) {
      return m;
    }
    return {
      ...m,
      blocks: m.blocks.map((b) =>
        b.type === "confirm" && b.card.action_uuid === actionUuid
          ? { ...b, card: { ...b.card, status } }
          : b,
      ),
    };
  });
}

/** Text of a message's text blocks (for the copy button / tests). */
export function messageText(message: ChatMessage): string {
  return message.blocks
    .filter((b): b is { type: "text"; text: string } => b.type === "text")
    .map((b) => b.text)
    .join("");
}

/** Remaining quota in percent, null when the quota is unlimited. */
export function quotaRemainingPercent(quota: {
  limit: number;
  used: number;
}): number | null {
  if (!quota.limit || quota.limit <= 0) return null;
  const left = Math.max(0, quota.limit - quota.used);
  return Math.round((left / quota.limit) * 100);
}
