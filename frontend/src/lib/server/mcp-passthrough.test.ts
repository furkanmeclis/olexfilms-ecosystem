import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/config/api", () => ({
  upstreamConfig: { baseUrl: "http://go.internal:8080/v1" },
}));

import { apiOrigin, mcpPassthrough } from "./mcp-passthrough";

type Captured = { url: string; init: RequestInit };
let captured: Captured[] = [];
let respond: () => Response;

beforeEach(() => {
  captured = [];
  respond = () =>
    new Response(JSON.stringify({ ok: true }), {
      status: 200,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-store",
        "Set-Cookie": "leak=1",
        "WWW-Authenticate": 'Bearer resource_metadata="x"',
      },
    });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      captured.push({ url, init });
      return respond();
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("MCP / OAuth passthrough (TEC-400)", () => {
  it("targets the Go origin without /v1 and keeps the query", async () => {
    expect(apiOrigin()).toBe("http://go.internal:8080");
    await mcpPassthrough(
      new Request("https://olexfilms.app/oauth/authorize?client_id=a&state=b"),
      "/oauth",
      ["authorize"],
    );
    expect(captured[0]!.url).toBe(
      "http://go.internal:8080/oauth/authorize?client_id=a&state=b",
    );
    expect(captured[0]!.init.redirect).toBe("manual");
  });

  it("forwards Bearer and MCP headers, never cookies", async () => {
    await mcpPassthrough(
      new Request("https://olexfilms.app/mcp/dealer", {
        method: "POST",
        headers: {
          Authorization: "Bearer tok",
          Cookie: "session=secret",
          "Content-Type": "application/json",
          "Mcp-Session-Id": "s1",
          "Mcp-Protocol-Version": "2025-06-18",
          "X-Real-IP": "203.0.113.5",
          Host: "olexfilms.app",
        },
        body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "ping" }),
      }),
      "/mcp",
      ["dealer"],
    );
    const h = new Headers(captured[0]!.init.headers);
    expect(h.get("authorization")).toBe("Bearer tok");
    expect(h.get("mcp-session-id")).toBe("s1");
    expect(h.get("mcp-protocol-version")).toBe("2025-06-18");
    expect(h.get("x-forwarded-for")).toBe("203.0.113.5");
    expect(h.get("cookie")).toBeNull();
    const body = new TextDecoder().decode(
      captured[0]!.init.body as ArrayBuffer,
    );
    expect(JSON.parse(body).method).toBe("ping");
  });

  it("returns OAuth headers and drops Set-Cookie", async () => {
    const res = await mcpPassthrough(
      new Request("https://olexfilms.app/mcp/user"),
      "/mcp",
      ["user"],
    );
    expect(res.status).toBe(200);
    expect(res.headers.get("www-authenticate")).toContain("resource_metadata");
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(await res.json()).toEqual({ ok: true });
  });

  it("streams the upstream body (SSE)", async () => {
    const encoder = new TextEncoder();
    respond = () =>
      new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(encoder.encode("event: message\n"));
            controller.enqueue(encoder.encode("data: {}\n\n"));
            controller.close();
          },
        }),
        { status: 200, headers: { "Content-Type": "text/event-stream" } },
      );
    const res = await mcpPassthrough(
      new Request("https://olexfilms.app/mcp/dealer", {
        headers: { Accept: "text/event-stream" },
      }),
      "/mcp",
      ["dealer"],
    );
    expect(res.headers.get("content-type")).toBe("text/event-stream");
    expect(await res.text()).toBe("event: message\ndata: {}\n\n");
  });

  it("refuses path traversal and answers 503 when Go is down", async () => {
    const bad = await mcpPassthrough(
      new Request("https://olexfilms.app/oauth/x"),
      "/oauth",
      ["..", "v1"],
    );
    expect(bad.status).toBe(404);
    expect(captured).toHaveLength(0);

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("ECONNREFUSED");
      }),
    );
    const down = await mcpPassthrough(
      new Request(
        "https://olexfilms.app/.well-known/oauth-authorization-server",
      ),
      "/.well-known",
      ["oauth-authorization-server"],
    );
    expect(down.status).toBe(503);
    expect((await down.json()).error).toBe("temporarily_unavailable");
  });
});
