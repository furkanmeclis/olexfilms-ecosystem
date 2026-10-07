import {
  portalRequest,
  portalStream,
} from "@/features/portal/lib/portal-client";
import { parseApiError } from "@/lib/api";

import {
  AssistantRequestError,
  type AIConversation,
  type AIConversationDetail,
  type AIStatus,
  type AssistantTransport,
  type LegalText,
} from "./types";

async function streamOrThrow(
  response: Response,
): Promise<ReadableStream<Uint8Array>> {
  if (response.ok && response.body) return response.body;
  type ErrorBody = { data?: { consent?: LegalText } } | null;
  const body = (await response.json().catch(() => null)) as ErrorBody;
  const err = parseApiError(response.status, body, { emitLimitEvent: false });
  throw new AssistantRequestError(
    err.message,
    response.status,
    err.code,
    body?.data?.consent ?? null,
  );
}

const BASE = "portal/ai";

/** Query-cache scope of the portal assistant. */
export const PORTAL_ASSISTANT_SCOPE = "portal";

/**
 * Portal assistant (`/v1/portal/ai/*`, customer realm, the brand center's
 * system pool). Customer tools only; confirmed actions have no panel page.
 */
export const portalTransport: AssistantTransport = {
  realm: "portal",
  status() {
    return portalRequest<AIStatus>(`${BASE}/status`);
  },
  async listConversations() {
    const page = await portalRequest<{ items: AIConversation[] }>(
      `${BASE}/conversations?limit=50&sort=-updated_at`,
    );
    return page.items;
  },
  getConversation(uuid) {
    return portalRequest<AIConversationDetail>(
      `${BASE}/conversations/${encodeURIComponent(uuid)}`,
    );
  },
  createConversation() {
    return portalRequest<AIConversation>(`${BASE}/conversations`, {
      method: "POST",
      body: {},
    });
  },
  renameConversation(uuid, title) {
    return portalRequest<AIConversation>(
      `${BASE}/conversations/${encodeURIComponent(uuid)}`,
      { method: "PATCH", body: { title } },
    );
  },
  async deleteConversation(uuid) {
    await portalRequest(`${BASE}/conversations/${encodeURIComponent(uuid)}`, {
      method: "DELETE",
    });
  },
  async acceptConsent(text) {
    await portalRequest("portal/consents", {
      method: "POST",
      body: {
        kind: text.kind,
        locale: text.locale,
        version: text.version,
        accepted: true,
      },
    });
  },
  async sendMessage(conversation, content, signal) {
    return streamOrThrow(
      await portalStream(
        `${BASE}/conversations/${encodeURIComponent(conversation)}/messages`,
        { body: { content }, signal },
      ),
    );
  },
  async confirmAction(action, edits, signal) {
    return streamOrThrow(
      await portalStream(
        `${BASE}/actions/${encodeURIComponent(action)}/confirm`,
        { body: edits ? { edits } : {}, signal },
      ),
    );
  },
  async cancelAction(action, signal) {
    return streamOrThrow(
      await portalStream(
        `${BASE}/actions/${encodeURIComponent(action)}/cancel`,
        { signal },
      ),
    );
  },
  recordHref() {
    return null;
  },
};
