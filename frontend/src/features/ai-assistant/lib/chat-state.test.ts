import { describe, expect, it } from "vitest";

import {
  applyStreamEvent,
  messageText,
  quotaRemainingPercent,
} from "./chat-state";
import { parseEventBlock, readEventStream } from "./sse";
import type { ChatMessage, StreamEvent } from "./types";

function streamOf(chunks: string[]) {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
}

describe("SSE parsing", () => {
  it("reads events split across chunks and skips pings", async () => {
    const events: StreamEvent[] = [];
    await readEventStream(
      streamOf([
        ': ping\n\nevent: text_delta\ndata: {"te',
        'xt":"Mer"}\n\nevent: text_delta\r\ndata: {"text":"haba"}\r\n\r\n',
        'event: message_done\ndata: {"message_uuid":"m1","status":"complete"}',
      ]),
      (e) => events.push(e),
    );
    expect(events.map((e) => e.event)).toEqual([
      "text_delta",
      "text_delta",
      "message_done",
    ]);
    expect(events[1]).toEqual({ event: "text_delta", data: { text: "haba" } });
  });

  it("ignores blocks without JSON data", () => {
    expect(parseEventBlock(": ping")).toBeNull();
    expect(parseEventBlock("event: x\ndata: {bad")).toBeNull();
  });
});

describe("applyStreamEvent", () => {
  const draft: ChatMessage = {
    id: "local",
    role: "assistant",
    status: "pending",
    blocks: [],
  };

  it("joins text_delta parts into one text block of one message", () => {
    let state = { messages: [draft], id: "local" };
    const events: StreamEvent[] = [
      {
        event: "message_start",
        data: { conversation_uuid: "c1", message_uuid: "m1" },
      },
      { event: "text_delta", data: { text: "Mer" } },
      { event: "text_delta", data: { text: "haba " } },
      { event: "text_delta", data: { text: "dünya" } },
      {
        event: "message_done",
        data: { message_uuid: "m1", status: "complete" },
      },
    ];
    for (const e of events)
      state = applyStreamEvent(state.messages, state.id, e);
    expect(state.messages).toHaveLength(1);
    expect(state.messages[0]!.id).toBe("m1");
    expect(state.messages[0]!.status).toBe("complete");
    expect(state.messages[0]!.blocks).toEqual([
      { type: "text", text: "Merhaba dünya" },
    ]);
    expect(messageText(state.messages[0]!)).toBe("Merhaba dünya");
  });

  it("splits text around a tool call and closes the tool chip", () => {
    let state = { messages: [draft], id: "local" };
    const events: StreamEvent[] = [
      { event: "text_delta", data: { text: "Bakıyorum." } },
      { event: "tool_start", data: { id: "t1", name: "search_customers" } },
      {
        event: "tool_result",
        data: { id: "t1", name: "search_customers", ok: true },
      },
      { event: "text_delta", data: { text: "Buldum." } },
    ];
    for (const e of events)
      state = applyStreamEvent(state.messages, state.id, e);
    expect(state.messages[0]!.blocks).toEqual([
      { type: "text", text: "Bakıyorum." },
      { type: "tool", id: "t1", name: "search_customers", status: "done" },
      { type: "text", text: "Buldum." },
    ]);
  });

  it("an action outcome closes the card it answers", () => {
    const card = {
      action_uuid: "a1",
      tool_use_id: "tu",
      tool_name: "create_task",
      source: "panel" as const,
      status: "pending" as const,
      preview: {},
      expires_at: "2099-01-01T00:00:00Z",
      created_at: "2026-01-01T00:00:00Z",
    };
    const withCard: ChatMessage = {
      ...draft,
      id: "m1",
      status: "complete",
      blocks: [{ type: "confirm", card }],
    };
    const next: ChatMessage = { ...draft, id: "m2" };
    const out = applyStreamEvent([withCard, next], "m2", {
      event: "action",
      data: {
        action_uuid: "a1",
        tool_use_id: "tu",
        tool_name: "create_task",
        source: "panel",
        status: "confirmed",
      },
    });
    const first = out.messages[0]!.blocks[0]!;
    expect(first.type === "confirm" && first.card.status).toBe("confirmed");
    expect(out.messages[1]!.blocks[0]!.type).toBe("action");
  });
});

describe("quotaRemainingPercent", () => {
  it("is null for an unlimited quota and rounds the share left", () => {
    expect(quotaRemainingPercent({ limit: 0, used: 10 })).toBeNull();
    expect(quotaRemainingPercent({ limit: 200, used: 50 })).toBe(75);
    expect(quotaRemainingPercent({ limit: 100, used: 150 })).toBe(0);
  });
});
