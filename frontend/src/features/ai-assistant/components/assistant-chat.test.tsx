// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  permissions: new Set<string>(["ai.actions.confirm"]),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (k: string, p?: Record<string, string | number>) =>
      p ? `${k}:${Object.values(p).join(",")}` : k,
    locale: "tr",
    dir: "ltr",
    format: { dateTime: (v: string) => v },
  }),
}));

vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.permissions.has(p) }),
}));

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: unknown }) =>
    createElement("a", { href }, children as never),
}));

import type { AIStatus, AssistantTransport, LegalText } from "../lib/types";
import { AssistantChat } from "./assistant-chat";

const CONSENT: LegalText = {
  uuid: "lt1",
  kind: "ai_guidelines",
  locale: "tr",
  version: 2,
  body: "## Yönerge\n\n- Kişisel veri paylaşmayın.",
  created_at: "2026-10-01T00:00:00Z",
};

function status(over: Partial<AIStatus> = {}): AIStatus {
  return {
    enabled: true,
    allowed: true,
    consent_required: false,
    quota: { period: "2026-10", limit: 1000, used: 250, remaining: 750 },
    ...over,
  };
}

type Ev = [string, unknown];

function sse(events: Ev[]) {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const [event, data] of events) {
        controller.enqueue(
          encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`),
        );
      }
      controller.close();
    },
  });
}

const CARD = {
  action_uuid: "11111111-1111-1111-1111-111111111111",
  tool_use_id: "tu1",
  tool_name: "create_task",
  source: "panel",
  status: "pending",
  preview: {
    action: "create_task",
    summary: 'Acme için "Müşteriyi ara" görevi oluşturulsun.',
    fields: [{ key: "title", value: "Müşteriyi ara" }],
    edit: [
      { key: "title", type: "text", value: "Müşteriyi ara", required: true },
    ],
  },
  expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
  created_at: new Date().toISOString(),
};

function fakeTransport(
  over: Partial<AssistantTransport> = {},
): AssistantTransport & Record<string, ReturnType<typeof vi.fn>> {
  return {
    realm: "panel",
    status: vi.fn(async () => status()),
    listConversations: vi.fn(async () => []),
    getConversation: vi.fn(),
    createConversation: vi.fn(async () => ({
      uuid: "c1",
      title: "",
      message_count: 0,
      last_message_at: null,
      created_at: "2026-10-07T10:00:00Z",
      updated_at: "2026-10-07T10:00:00Z",
    })),
    renameConversation: vi.fn(),
    deleteConversation: vi.fn(),
    acceptConsent: vi.fn(async () => undefined),
    sendMessage: vi.fn(async () => sse([])),
    confirmAction: vi.fn(async () => sse([])),
    cancelAction: vi.fn(async () => sse([])),
    recordHref: vi.fn(() => null),
    ...over,
  } as never;
}

let root: Root;
let host: HTMLDivElement;

async function flush() {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
}

async function mount(transport: AssistantTransport, showToolChips = true) {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    // Strict Mode runs state updaters twice, like `next dev`.
    root.render(
      createElement(
        StrictMode,
        null,
        createElement(
          QueryClientProvider,
          { client },
          createElement(AssistantChat, {
            transport,
            scope: "test",
            showToolChips,
          }),
        ),
      ),
    );
  });
  await flush();
}

const q = (id: string) =>
  document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const qa = (id: string) =>
  Array.from(document.querySelectorAll<HTMLElement>(`[data-testid="${id}"]`));

async function type(text: string) {
  const input = q("ai-input") as HTMLTextAreaElement;
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )!.set!;
  await act(async () => {
    setter.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function send(text: string) {
  await type(text);
  await act(async () => {
    q("ai-send")!.click();
  });
  await flush();
}

describe("AssistantChat", () => {
  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    state.permissions = new Set(["ai.actions.confirm"]);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    document.body.innerHTML = "";
  });

  it("joins text_delta parts into one assistant message", async () => {
    const transport = fakeTransport({
      sendMessage: vi.fn(async () =>
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m1" }],
          ["text_delta", { text: "Mer" }],
          ["text_delta", { text: "haba, " }],
          ["text_delta", { text: "**nasıl** yardımcı olabilirim?" }],
          ["message_done", { message_uuid: "m1", status: "complete" }],
        ]),
      ),
    });
    await mount(transport);
    await send("Selam");

    expect(transport.createConversation).toHaveBeenCalledTimes(1);
    expect(transport.sendMessage).toHaveBeenCalledWith(
      "c1",
      "Selam",
      expect.any(AbortSignal),
    );
    expect(qa("ai-message-user")).toHaveLength(1);
    const assistant = qa("ai-message-assistant");
    expect(assistant).toHaveLength(1);
    expect(
      assistant[0]!.querySelectorAll('[data-testid="ai-text"]'),
    ).toHaveLength(1);
    expect(assistant[0]!.textContent).toContain(
      "Merhaba, nasıl yardımcı olabilirim?",
    );
    expect(assistant[0]!.querySelector("strong")?.textContent).toBe("nasıl");
  });

  it("shows the confirm card and Onayla calls the confirm endpoint", async () => {
    const transport = fakeTransport({
      sendMessage: vi.fn(async () =>
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m1" }],
          ["text_delta", { text: "Görevi oluşturayım mı?" }],
          ["confirm", CARD],
        ]),
      ),
      confirmAction: vi.fn(async () =>
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m2" }],
          [
            "action",
            {
              action_uuid: CARD.action_uuid,
              tool_use_id: "tu1",
              tool_name: "create_task",
              source: "panel",
              status: "confirmed",
              link: { kind: "task", uuid: "t1" },
            },
          ],
          ["text_delta", { text: "Görev oluşturuldu." }],
          ["message_done", { message_uuid: "m2", status: "complete" }],
        ]),
      ),
      recordHref: vi.fn(() => "/t/acme/tasks/t1"),
    });
    await mount(transport);
    await send("Yarın için görev aç");

    const card = q("ai-confirm-card");
    expect(card).not.toBeNull();
    expect(card!.textContent).toContain("ai.actions.create_task");
    expect(card!.textContent).toContain("Müşteriyi ara");
    // TEC-461: the backend summary (user's locale) is shown on the card.
    expect(q("ai-card-summary")?.textContent).toBe(CARD.preview.summary);
    expect(q("ai-card-countdown")).not.toBeNull();

    await act(async () => {
      q("ai-card-confirm")!.click();
    });
    await flush();

    expect(transport.confirmAction).toHaveBeenCalledWith(
      CARD.action_uuid,
      undefined,
      expect.any(AbortSignal),
    );
    expect(q("ai-card-confirm")).toBeNull();
    const outcome = q("ai-action-outcome");
    expect(outcome?.textContent).toContain("ai.action.confirmed");
    expect(outcome?.querySelector("a")?.getAttribute("href")).toBe(
      "/t/acme/tasks/t1",
    );
  });

  it("hides the confirm button and shows guidance without ai.actions.confirm", async () => {
    state.permissions.delete("ai.actions.confirm");
    const transport = fakeTransport({
      sendMessage: vi.fn(async () =>
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m1" }],
          ["confirm", CARD],
        ]),
      ),
    });
    await mount(transport);
    await send("Görev aç");

    expect(q("ai-confirm-card")).not.toBeNull();
    expect(q("ai-card-confirm")).toBeNull();
    expect(q("ai-card-confirm-permission")?.textContent).toContain(
      "ai.card.confirm_permission",
    );
    expect(transport.confirmAction).not.toHaveBeenCalled();
  });

  it("sends edited card fields with the confirmation", async () => {
    const transport = fakeTransport({
      sendMessage: vi.fn(async () =>
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m1" }],
          ["confirm", CARD],
        ]),
      ),
    });
    await mount(transport);
    await send("Görev aç");

    const input = document.getElementById(
      `ai-edit-${CARD.action_uuid}-title`,
    ) as HTMLInputElement;
    const setter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!;
    await act(async () => {
      setter.call(input, "Müşteriyi yarın ara");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      q("ai-card-confirm")!.click();
    });
    await flush();
    expect(transport.confirmAction).toHaveBeenCalledWith(
      CARD.action_uuid,
      { title: "Müşteriyi yarın ara" },
      expect.any(AbortSignal),
    );
  });

  it("guidelines dialog starts unchecked; Accept stays disabled until checked", async () => {
    const transport = fakeTransport({
      status: vi.fn(async () =>
        status({ consent_required: true, consent: CONSENT }),
      ),
    });
    await mount(transport);

    expect(q("ai-assistant-consent")?.textContent).toContain("Yönerge");
    const box = document.getElementById("ai-assistant-consent-accept");
    expect(box?.getAttribute("aria-checked")).toBe("false");
    const accept = q("ai-assistant-consent-accept") as HTMLButtonElement;
    expect(accept.disabled).toBe(true);
    expect((q("ai-input") as HTMLTextAreaElement).disabled).toBe(true);

    await act(async () => {
      accept.click();
    });
    expect(transport.acceptConsent).not.toHaveBeenCalled();

    await act(async () => {
      box!.click();
    });
    expect(accept.disabled).toBe(false);
    (transport.status as ReturnType<typeof vi.fn>).mockResolvedValue(status());
    await act(async () => {
      accept.click();
    });
    await flush();
    expect(transport.acceptConsent).toHaveBeenCalledWith(CONSENT);
    expect(q("ai-assistant-consent")).toBeNull();
    expect((q("ai-input") as HTMLTextAreaElement).disabled).toBe(false);
  });

  it("a 428 answer opens the guidelines dialog and keeps the text", async () => {
    const { AssistantRequestError } = await import("../lib/types");
    const transport = fakeTransport({
      sendMessage: vi.fn(async () => {
        throw new AssistantRequestError(
          "consent",
          428,
          "AI_CONSENT_REQUIRED",
          CONSENT,
        );
      }),
    });
    await mount(transport);
    await send("Selam");
    expect(q("ai-assistant-consent")).not.toBeNull();
    expect((q("ai-input") as HTMLTextAreaElement).value).toBe("Selam");
    expect((q("ai-input") as HTMLTextAreaElement).disabled).toBe(true);
    expect(qa("ai-message-user")).toHaveLength(0);
  });

  it("quota_exceeded shows the band and disables the input", async () => {
    const transport = fakeTransport({
      sendMessage: vi.fn(async () =>
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m1" }],
          ["text_delta", { text: "Kısaca:" }],
          ["quota_exceeded", { limit: 1000, used: 1000 }],
          ["message_done", { message_uuid: "m1", status: "complete" }],
        ]),
      ),
    });
    await mount(transport);
    expect(q("ai-quota")?.textContent).toContain("ai.quota.remaining:75");
    expect(q("ai-quota-band")).toBeNull();
    await send("Rapor");
    expect(q("ai-quota-band")).not.toBeNull();
    expect((q("ai-input") as HTMLTextAreaElement).disabled).toBe(true);
    expect((q("ai-send") as HTMLButtonElement).disabled).toBe(true);
  });

  it("an error event offers retry, which repeats the request", async () => {
    const sendMessage = vi
      .fn()
      .mockResolvedValueOnce(
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m1" }],
          ["error", { code: "provider_error", message: "upstream" }],
        ]),
      )
      .mockResolvedValueOnce(
        sse([
          ["message_start", { conversation_uuid: "c1", message_uuid: "m2" }],
          ["text_delta", { text: "Tamam." }],
          ["message_done", { message_uuid: "m2", status: "complete" }],
        ]),
      );
    const transport = fakeTransport({ sendMessage });
    await mount(transport);
    await send("Selam");
    expect(q("ai-error")?.textContent).toContain("ai.errors.provider_error");
    await act(async () => {
      q("ai-retry")!.click();
    });
    await flush();
    expect(sendMessage).toHaveBeenCalledTimes(2);
    expect(sendMessage.mock.calls[1]![1]).toBe("Selam");
    expect(q("ai-error")).toBeNull();
    expect(qa("ai-message-user")).toHaveLength(1);
  });

  it("Stop aborts the running request", async () => {
    let signal: AbortSignal | null = null;
    const transport = fakeTransport({
      sendMessage: vi.fn(async (_c: string, _t: string, s: AbortSignal) => {
        signal = s;
        return new ReadableStream<Uint8Array>({
          start(controller) {
            s.addEventListener("abort", () =>
              controller.error(new DOMException("aborted", "AbortError")),
            );
          },
        });
      }),
    });
    await mount(transport);
    await send("Uzun bir soru");
    expect(q("ai-stop")).not.toBeNull();
    await act(async () => {
      q("ai-stop")!.click();
    });
    await flush();
    expect(signal!.aborted).toBe(true);
    expect(q("ai-stop")).toBeNull();
    expect(q("ai-send")).not.toBeNull();
  });

  it("panel shows tool chips; the portal variant hides them", async () => {
    const events: Ev[] = [
      ["message_start", { conversation_uuid: "c1", message_uuid: "m1" }],
      ["tool_start", { id: "t1", name: "search_customers" }],
      ["tool_result", { id: "t1", name: "search_customers", ok: true }],
      ["text_delta", { text: "3 müşteri buldum." }],
      ["message_done", { message_uuid: "m1", status: "complete" }],
    ];
    const panel = fakeTransport({
      sendMessage: vi.fn(async () => sse(events)),
    });
    await mount(panel, true);
    await send("Müşteriler");
    expect(q("ai-tool-chip")?.textContent).toContain(
      "ai.tools.search_customers",
    );
    act(() => root.unmount());
    document.body.innerHTML = "";

    const portal = fakeTransport({
      realm: "portal",
      sendMessage: vi.fn(async () => sse(events)),
    });
    await mount(portal, false);
    await send("Araçlarım");
    expect(q("ai-tool-chip")).toBeNull();
    expect(q("ai-message-assistant")?.textContent).toContain(
      "3 müşteri buldum.",
    );
  });

  it("shows the unavailable notice when the module is off", async () => {
    const transport = fakeTransport({
      status: vi.fn(async () => status({ enabled: false })),
    });
    await mount(transport);
    expect(q("ai-unavailable")).not.toBeNull();
    expect(q("ai-input")).toBeNull();
  });
});
