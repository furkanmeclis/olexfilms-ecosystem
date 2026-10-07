import type { components } from "@/generated/api";

export type AIStatus = components["schemas"]["AIStatus"];
export type AIConversation = components["schemas"]["AIConversation"];
export type AIConversationDetail =
  components["schemas"]["AIConversationDetail"];
export type AIMessage = components["schemas"]["AIMessage"];
export type AIActionCard = components["schemas"]["AIActionCard"];
export type AIActionOutcome = components["schemas"]["AIActionOutcome"];
export type AIUIBlock = components["schemas"]["AIUIBlock"];
export type LegalText = components["schemas"]["LegalText"];

/** `preview` of an action card (TEC-387 tools.Preview). */
export type ActionPreview = {
  action?: string;
  summary?: string;
  fields?: { key: string; value: string }[];
  edit?: {
    key: string;
    type: "text" | "textarea" | "date" | "time" | "select" | "number";
    value: string;
    options?: string[];
    required?: boolean;
  }[];
  warnings?: string[];
};

/** One rendered block of a chat message (same shape as AIUIBlock). */
export type ChatBlock =
  | { type: "text"; text: string }
  | {
      type: "tool";
      id: string;
      name: string;
      status: "running" | "done" | "error" | "pending";
    }
  | { type: "confirm"; card: AIActionCard }
  | { type: "action"; action: AIActionOutcome }
  | { type: "error"; code: string; message?: string };

export type ChatMessage = {
  /** Server uuid, or a local id until `message_start` arrives. */
  id: string;
  role: "user" | "assistant";
  status: "pending" | "complete" | "error" | "cancelled";
  blocks: ChatBlock[];
};

/** One parsed server-sent event of a turn (TEC-388). */
export type StreamEvent =
  | {
      event: "message_start";
      data: {
        conversation_uuid: string;
        message_uuid: string;
        user_message_uuid?: string;
      };
    }
  | { event: "text_delta"; data: { text: string } }
  | { event: "tool_start"; data: { id: string; name: string } }
  | {
      event: "tool_result";
      data: { id: string; name: string; ok: boolean; code?: string };
    }
  | { event: "confirm"; data: AIActionCard }
  | { event: "action"; data: AIActionOutcome }
  | { event: "quota_exceeded"; data: { limit: number; used: number } }
  | { event: "error"; data: { code: string; message?: string } }
  | {
      event: "message_done";
      data: { message_uuid: string; status: string; stop_reason?: string };
    }
  | { event: "title"; data: { conversation_uuid: string; title: string } };

/** A failed request before the stream started (non-2xx answer). */
export class AssistantRequestError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    /** `data.consent` of a 428 AI_CONSENT_REQUIRED answer. */
    readonly consent: LegalText | null = null,
  ) {
    super(message);
    this.name = "AssistantRequestError";
  }
}

/**
 * Everything the chat needs from the API. The panel (`/v1/ai/*`, active
 * organization) and the portal (`/v1/portal/ai/*`, customer realm) each
 * provide one; the UI is shared.
 */
export type AssistantTransport = {
  realm: "panel" | "portal";
  status(): Promise<AIStatus>;
  listConversations(): Promise<AIConversation[]>;
  getConversation(uuid: string): Promise<AIConversationDetail>;
  createConversation(): Promise<AIConversation>;
  renameConversation(uuid: string, title: string): Promise<AIConversation>;
  deleteConversation(uuid: string): Promise<void>;
  acceptConsent(text: LegalText): Promise<void>;
  /** Streams a new user message; throws AssistantRequestError when refused. */
  sendMessage(
    conversation: string,
    content: string,
    signal: AbortSignal,
  ): Promise<ReadableStream<Uint8Array>>;
  confirmAction(
    action: string,
    edits: Record<string, unknown> | undefined,
    signal: AbortSignal,
  ): Promise<ReadableStream<Uint8Array>>;
  cancelAction(
    action: string,
    signal: AbortSignal,
  ): Promise<ReadableStream<Uint8Array>>;
  /** Link of a record a confirmed action created (null: no page). */
  recordHref(kind: string, uuid: string): string | null;
};
