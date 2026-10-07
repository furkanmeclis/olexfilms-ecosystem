// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  Conversation,
  ConversationMessage,
} from "@/features/conversations/services/conversations.service";
import type { RealtimePublicationHandler } from "@/lib/realtime/types";

const state = vi.hoisted(() => ({
  permissions: new Set<string>([
    "conversations.read",
    "conversations.reply",
    "conversations.manage",
  ]),
  channelHandler: null as null | RealtimePublicationHandler,
  channel: null as null | string,
}));

const service = vi.hoisted(() => ({
  get: vi.fn(),
  messages: vi.fn(),
  aiRuns: vi.fn(),
  reply: vi.fn(),
  patch: vi.fn(),
  markRead: vi.fn(),
  mediaUrl: (c: string, m: string) =>
    `/api/v1/conversations/${c}/messages/${m}/media`,
}));

vi.mock("@/features/conversations/services/conversations.service", () => ({
  conversationsService: service,
}));
vi.mock("@/features/users/services/users.service", () => ({
  usersService: {
    list: vi.fn(async () => ({ items: [], total: 0, limit: 100, offset: 0 })),
  },
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    dir: "ltr",
    format: {
      dateTime: (v: string) => v,
      time: (v: string) => v,
      relative: (v: string) => v,
      number: (v: number) => String(v),
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.permissions.has(p) }),
}));
vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({ user: { uuid: "me", fullName: "Ada Admin" } }),
}));
vi.mock("@/hooks/use-realtime", () => ({
  useChannel: (
    channel: string,
    handler: RealtimePublicationHandler,
    options?: { enabled?: boolean },
  ) => {
    if (options?.enabled === false) return;
    state.channel = channel;
    state.channelHandler = handler;
  },
}));
vi.mock("@/config/realtime", async (orig) => {
  const mod = await orig<typeof import("@/config/realtime")>();
  return { realtimeConfig: { ...mod.realtimeConfig, enabled: true } };
});

import { ConversationPanel } from "./conversation-panel";
import { useConversationsRealtime } from "@/features/conversations/hooks/use-conversations-realtime";
import { MAX_ATTACHMENT_BYTES } from "@/features/conversations/lib/conversations";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const UUID = "0b9c4c1e-0000-4000-8000-0000000000a1";
const NOW = "2026-10-07T09:00:00Z";

const conversation: Conversation = {
  uuid: UUID,
  channel: "whatsapp",
  contact_e164: "+905551112233",
  contact_name: "Ayşe Yılmaz",
  status: "open",
  ai_mode: "auto",
  ai_paused_until: null,
  assigned_user: null,
  assigned_org: null,
  identity_kind: "customer",
  identity_user: null,
  identity_org: null,
  locale: "tr",
  unread_count: 0,
  last_message_at: NOW,
  last_inbound_at: NOW,
  ai_consent_at: NOW,
  created_at: NOW,
  updated_at: NOW,
};

function message(over: Partial<ConversationMessage>): ConversationMessage {
  return {
    uuid: "m-1",
    conversation_uuid: UUID,
    direction: "in",
    sender_type: "contact",
    status: "received",
    body: "Merhaba",
    has_stored_media: false,
    send_attempts: 0,
    created_at: NOW,
    sender_user: null,
    ...over,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  state.permissions = new Set([
    "conversations.read",
    "conversations.reply",
    "conversations.manage",
  ]);
  state.channelHandler = null;
  state.channel = null;
  service.get.mockReset().mockResolvedValue(conversation);
  service.messages.mockReset().mockResolvedValue({
    items: [message({})],
    next_cursor: null,
  });
  service.aiRuns.mockReset().mockResolvedValue({
    items: [
      {
        uuid: "run-1",
        status: "completed",
        model: "claude-sonnet-5-5",
        tokens: 42,
        input_tokens: 20,
        output_tokens: 22,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        duration_ms: 1300,
        error: null,
        created_at: NOW,
      },
    ],
  });
  service.reply.mockReset();
  service.patch.mockReset().mockImplementation(async (_uuid, body) => ({
    ...conversation,
    ...body,
  }));
  service.markRead.mockReset().mockResolvedValue(conversation);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function Realtime({ children }: { children: ReactNode }) {
  useConversationsRealtime(true);
  return children;
}

async function flush() {
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(
          Realtime,
          null,
          createElement(ConversationPanel, {
            uuid: UUID,
            initial: conversation,
          }),
        ),
      ),
    );
  });
  await flush();
}

const q = <T extends Element = HTMLElement>(selector: string) =>
  container.querySelector<T & Element>(selector) as T | null;

function typeInto(el: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )!.set!;
  setter.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

function pickFile(file: File) {
  const input = q<HTMLInputElement>('[data-testid="attachment-input"]')!;
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

const bubbles = (sender: string) =>
  Array.from(
    container.querySelectorAll(
      `[data-testid="message"][data-sender="${sender}"]`,
    ),
  );

describe("ConversationPanel", () => {
  it("adds the staff bubble optimistically when a reply is sent", async () => {
    service.reply.mockImplementation(() => new Promise(() => {}));
    await render();
    expect(bubbles("contact")).toHaveLength(1);

    await act(async () => {
      typeInto(q('[data-testid="composer-input"]')!, "Randevunuz hazır");
    });
    await act(async () => {
      q<HTMLButtonElement>('[data-testid="composer-send"]')!.click();
    });
    await flush();

    expect(service.reply).toHaveBeenCalledWith(UUID, "Randevunuz hazır", null);
    const staff = bubbles("staff");
    expect(staff).toHaveLength(1);
    expect(staff[0].textContent).toContain("Randevunuz hazır");
    expect(staff[0].textContent).toContain("Ada Admin");
    expect(
      staff[0]
        .querySelector('[data-testid="message-status"]')
        ?.getAttribute("data-status"),
    ).toBe("queued");
  });

  it("replaces the optimistic bubble and shows the AI pause note", async () => {
    service.reply.mockImplementation(async (_uuid: string, body: string) => ({
      conversation: { ...conversation, ai_mode: "paused" },
      message: message({
        uuid: "m-2",
        direction: "out",
        sender_type: "staff",
        status: "queued",
        body,
        sender_user: { uuid: "me", name: "Ada Admin" },
      }),
    }));
    await render();
    await act(async () => {
      typeInto(q('[data-testid="composer-input"]')!, "Tamam");
    });
    await act(async () => {
      q<HTMLButtonElement>('[data-testid="composer-send"]')!.click();
    });
    await flush();
    const staff = bubbles("staff");
    expect(staff).toHaveLength(1);
    expect(staff[0].getAttribute("data-uuid")).toBe("m-2");
    expect(q('[data-testid="ai-paused-notice"]')).not.toBeNull();
  });

  it("rejects an attachment over 16 MB without calling the API", async () => {
    await render();
    const big = new File(["x"], "scan.pdf", { type: "application/pdf" });
    Object.defineProperty(big, "size", { value: MAX_ATTACHMENT_BYTES + 1 });
    await act(async () => pickFile(big));

    const error = q('[data-testid="attachment-error"]');
    expect(error?.textContent).toBe("conversations.composer.file_too_large");
    expect(q('[data-testid="attachment-name"]')).toBeNull();
    expect(
      q<HTMLButtonElement>('[data-testid="composer-send"]')!.disabled,
    ).toBe(true);
    expect(service.reply).not.toHaveBeenCalled();

    const ok = new File(["x"], "photo.png", { type: "image/png" });
    Object.defineProperty(ok, "size", { value: MAX_ATTACHMENT_BYTES });
    await act(async () => pickFile(ok));
    expect(q('[data-testid="attachment-error"]')).toBeNull();
    expect(q('[data-testid="attachment-name"]')?.textContent).toBe("photo.png");
  });

  it("PATCHes the conversation when the AI mode changes", async () => {
    await render();
    await act(async () => {
      q<HTMLButtonElement>('[data-testid="ai-mode-off"]')!.click();
    });
    await flush();
    expect(service.patch).toHaveBeenCalledWith(UUID, { ai_mode: "off" });
  });

  it("hides the composer without conversations.reply", async () => {
    state.permissions.delete("conversations.reply");
    await render();
    expect(q('[data-testid="message-thread"]')).not.toBeNull();
    expect(q('[data-testid="message-composer"]')).toBeNull();
  });

  it("opens the AI run drawer and lists recent runs", async () => {
    await render();
    await act(async () => {
      q<HTMLButtonElement>('[data-testid="ai-runs-open"]')!.click();
    });
    await flush();

    expect(service.aiRuns).toHaveBeenCalledWith(UUID);
    expect(document.body.textContent).toContain("claude-sonnet-5-5");
    expect(document.body.textContent).toContain("conversations.ai_runs.tokens");
    expect(
      document.querySelectorAll('[data-testid="ai-run-row"]'),
    ).toHaveLength(1);
  });

  it("appends a message from the realtime channel", async () => {
    await render();
    expect(state.channel).toBe("system.conversations");
    await act(async () => {
      state.channelHandler?.(
        {
          type: "conversations.message.created",
          data: {
            conversation_uuid: UUID,
            message: {
              uuid: "m-live",
              conversation_uuid: UUID,
              direction: "in",
              sender_type: "contact",
              status: "received",
              body: "Fiyat nedir?",
              has_stored_media: false,
              send_attempts: 0,
              created_at: "2026-10-07T09:05:00Z",
            },
          },
        },
        { channel: "system.conversations" },
      );
    });
    await flush();
    const contact = bubbles("contact");
    expect(contact).toHaveLength(2);
    expect(contact[1].textContent).toContain("Fiyat nedir?");

    // A status update replaces the bubble instead of adding one.
    await act(async () => {
      state.channelHandler?.(
        {
          type: "conversations.message.updated",
          data: {
            conversation_uuid: UUID,
            message: {
              uuid: "m-live",
              direction: "in",
              sender_type: "contact",
              status: "read",
              body: "Fiyat nedir?",
              created_at: "2026-10-07T09:05:00Z",
            },
          },
        },
        { channel: "system.conversations" },
      );
    });
    await flush();
    expect(bubbles("contact")).toHaveLength(2);
  });
});
