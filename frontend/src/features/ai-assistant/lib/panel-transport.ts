import { routes } from "@/config/routes";
import { apiClient, parseApiError, unwrap } from "@/lib/api";

import {
  AssistantRequestError,
  type AIConversation,
  type AIConversationDetail,
  type AIStatus,
  type AssistantTransport,
  type LegalText,
} from "./types";

type StreamResult = {
  data?: unknown;
  error?: unknown;
  response: Response;
};

/** Turns a refused stream request into an AssistantRequestError. */
export function streamOrThrow(
  result: StreamResult,
): ReadableStream<Uint8Array> {
  const { response } = result;
  if (response.ok && response.body) return response.body;
  if (response.ok && result.data instanceof ReadableStream) {
    return result.data as ReadableStream<Uint8Array>;
  }
  const body = (result.error ?? null) as {
    data?: { consent?: LegalText };
  } | null;
  const err = parseApiError(response.status, body, { emitLimitEvent: false });
  throw new AssistantRequestError(
    err.message,
    response.status,
    err.code,
    body?.data?.consent ?? null,
  );
}

const SSE_HEADERS = { Accept: "text/event-stream" };

/** Record pages of the panel a confirmed action can link to. */
export function panelRecordHref(
  slug: string,
  kind: string,
  uuid: string,
): string | null {
  switch (kind) {
    case "task":
      return routes.tenant.tasks.detail(slug, uuid);
    case "lead":
      return routes.tenant.leads.detail(slug, uuid);
    case "order":
      return routes.tenant.orders.detail(slug, uuid);
    case "service":
      return routes.tenant.services.detail(slug, uuid);
    case "appointment":
      return routes.tenant.appointments.calendar(slug);
    default:
      return null;
  }
}

/** Panel assistant (`/v1/ai/*`) of the tenant route's organization. */
export function createPanelTransport(slug: string): AssistantTransport {
  return {
    realm: "panel",
    async status() {
      return unwrap<AIStatus>(await apiClient.GET("/v1/ai/status"), {
        silent: true,
      });
    },
    async listConversations() {
      const page = await unwrap<{ items: AIConversation[] }>(
        await apiClient.GET("/v1/ai/conversations", {
          params: { query: { limit: 50, sort: "-updated_at" } },
        }),
        { silent: true },
      );
      return page.items;
    },
    async getConversation(uuid) {
      return unwrap<AIConversationDetail>(
        await apiClient.GET("/v1/ai/conversations/{uuid}", {
          params: { path: { uuid } },
        }),
      );
    },
    async createConversation() {
      return unwrap<AIConversation>(
        await apiClient.POST("/v1/ai/conversations", { body: {} }),
      );
    },
    async renameConversation(uuid, title) {
      return unwrap<AIConversation>(
        await apiClient.PATCH("/v1/ai/conversations/{uuid}", {
          params: { path: { uuid } },
          body: { title },
        }),
      );
    },
    async deleteConversation(uuid) {
      await unwrap(
        await apiClient.DELETE("/v1/ai/conversations/{uuid}", {
          params: { path: { uuid } },
        }),
      );
    },
    async acceptConsent(text) {
      await unwrap(
        await apiClient.POST("/v1/consents", {
          body: {
            kind: text.kind,
            locale: text.locale,
            version: text.version,
            accepted: true,
          },
        }),
      );
    },
    async sendMessage(conversation, content, signal) {
      return streamOrThrow(
        await apiClient.POST("/v1/ai/conversations/{uuid}/messages", {
          params: { path: { uuid: conversation } },
          body: { content },
          headers: SSE_HEADERS,
          parseAs: "stream",
          signal,
        }),
      );
    },
    async confirmAction(action, edits, signal) {
      return streamOrThrow(
        await apiClient.POST("/v1/ai/actions/{uuid}/confirm", {
          params: { path: { uuid: action } },
          body: edits ? { edits } : {},
          headers: SSE_HEADERS,
          parseAs: "stream",
          signal,
        }),
      );
    },
    async cancelAction(action, signal) {
      return streamOrThrow(
        await apiClient.POST("/v1/ai/actions/{uuid}/cancel", {
          params: { path: { uuid: action } },
          headers: SSE_HEADERS,
          parseAs: "stream",
          signal,
        }),
      );
    },
    recordHref(kind, uuid) {
      return panelRecordHref(slug, kind, uuid);
    },
  };
}
