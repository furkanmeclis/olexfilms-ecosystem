import type { ServerListParams } from "@/components/entity";
import type { ResourceMeta } from "@/features/io/types";
import type { components } from "@/generated/api";
import { apiConfig } from "@/config/api";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Conversation = Schemas["Conversation"];
export type ConversationMessage = Schemas["ConversationMessage"];
export type ConversationPatchInput = Schemas["ConversationPatchInput"];
export type ConversationStatus = Conversation["status"];
export type ConversationAIMode = Conversation["ai_mode"];
export type ConversationIdentityKind = Conversation["identity_kind"];
export type ConversationSenderType = ConversationMessage["sender_type"];
export type ConversationMessageStatus = ConversationMessage["status"];

export type ConversationReply = {
  conversation: Conversation;
  message: ConversationMessage;
};

/**
 * GET /v1/conversations params (TEC-398): `status`, `ai_mode`,
 * `identity_kind`, `assigned_user_uuid` are CSV, `unread` true|false,
 * `last_message_from` / `last_message_to` dates, `sort` one of
 * last_message_at, created_at, unread_count.
 */
export type ConversationListParams = ServerListParams & {
  status?: string;
  ai_mode?: string;
  identity_kind?: string;
  assigned_user_uuid?: string;
  channel?: string;
  unread?: string;
  last_message_from?: string;
  last_message_to?: string;
};

export type ConversationPage = {
  items: Conversation[];
  total: number;
  limit: number;
  offset: number;
};

export type ConversationMessagePage = {
  items: ConversationMessage[];
  next_cursor: string | null;
};

const enc = encodeURIComponent;

function withFile(body: string, file: File | null | undefined, extra = {}) {
  const form = new FormData();
  for (const [key, value] of Object.entries(extra)) {
    form.append(key, String(value));
  }
  if (body) form.append("body", body);
  if (file) form.append("file", file);
  return form;
}

export const conversationsService = {
  meta() {
    return platformRequest<ResourceMeta>("GET", "/v1/conversations/meta");
  },
  list(params: ConversationListParams) {
    return platformRequest<ConversationPage>("GET", "/v1/conversations", {
      query: params,
    });
  },
  get(uuid: string) {
    return platformRequest<Conversation>(
      "GET",
      `/v1/conversations/${enc(uuid)}`,
    );
  },
  patch(uuid: string, body: ConversationPatchInput) {
    return platformRequest<Conversation>(
      "PATCH",
      `/v1/conversations/${enc(uuid)}`,
      { body },
    );
  },
  messages(uuid: string, before?: string | null, limit = 50) {
    return platformRequest<ConversationMessagePage>(
      "GET",
      `/v1/conversations/${enc(uuid)}/messages`,
      { query: { before: before ?? undefined, limit } },
    );
  },
  /** Staff reply: JSON for text, multipart when a file is attached. */
  reply(uuid: string, body: string, file?: File | null) {
    const path = `/v1/conversations/${enc(uuid)}/messages`;
    if (file) {
      return platformFormRequest<ConversationReply>(
        "POST",
        path,
        withFile(body, file),
      );
    }
    return platformRequest<ConversationReply>("POST", path, {
      body: { body },
    });
  },
  /** Opens (or continues) the conversation of a user's number. */
  start(userUuid: string, body: string, file?: File | null) {
    if (file) {
      return platformFormRequest<ConversationReply>(
        "POST",
        "/v1/conversations",
        withFile(body, file, { user_uuid: userUuid }),
      );
    }
    return platformRequest<ConversationReply>("POST", "/v1/conversations", {
      body: { user_uuid: userUuid, body },
    });
  },
  markRead(uuid: string) {
    return platformRequest<Conversation>(
      "POST",
      `/v1/conversations/${enc(uuid)}/read`,
    );
  },
  /** BFF URL of a stored attachment (cookie-authenticated, usable as src). */
  mediaUrl(uuid: string, messageUuid: string) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    return `${base}/v1/conversations/${enc(uuid)}/messages/${enc(messageUuid)}/media`;
  },
};
