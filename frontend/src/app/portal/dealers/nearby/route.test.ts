import { afterEach, describe, expect, it, vi } from "vitest";

const upstream = vi.hoisted(() => ({ fetchUpstream: vi.fn() }));

vi.mock("@/lib/server/upstream", async (orig) => ({
  ...(await orig<object>()),
  fetchUpstream: upstream.fetchUpstream,
}));

import { GET } from "./route";

afterEach(() => {
  upstream.fetchUpstream.mockReset();
});

describe("GET /portal/dealers/nearby", () => {
  it("rejects a malformed query without calling Go", async () => {
    const res = await GET(
      new Request("https://olex.test/portal/dealers/nearby?lat=95&lng=29"),
    );
    expect(res.status).toBe(400);
    expect(await res.json()).toMatchObject({
      error: { code: "VALIDATION_ERROR" },
    });
    expect(upstream.fetchUpstream).not.toHaveBeenCalled();
  });

  it("forwards the query, client IP and host to Go", async () => {
    const body = new TextEncoder().encode(
      JSON.stringify({ success: true, data: { items: [] } }),
    );
    upstream.fetchUpstream.mockResolvedValue({
      status: 200,
      headers: new Headers(),
      body: body.buffer,
    });
    const res = await GET(
      new Request(
        "https://olex.test/portal/dealers/nearby?lat=41&lng=29&radius_km=50",
        {
          headers: {
            "x-real-ip": "203.0.113.9",
            "x-forwarded-host": "olexfilms.test",
            cookie: "session=secret",
          },
        },
      ),
    );
    expect(res.status).toBe(200);
    expect(res.headers.get("cache-control")).toBe("no-store");
    expect(await res.json()).toEqual({ success: true, data: { items: [] } });
    const [path, init] = upstream.fetchUpstream.mock.calls[0]!;
    expect(path).toBe(
      "public/dealers/nearby?lat=41.000000&lng=29.000000&radius_km=50",
    );
    const headers = new Headers(init.headers);
    expect(headers.get("x-forwarded-for")).toBe("203.0.113.9");
    expect(headers.get("x-forwarded-host")).toBe("olexfilms.test");
    expect(headers.get("cookie")).toBeNull();
  });

  it("passes 429 and Retry-After through", async () => {
    upstream.fetchUpstream.mockResolvedValue({
      status: 429,
      headers: new Headers({ "retry-after": "30" }),
      body: new TextEncoder().encode("{}").buffer,
    });
    const res = await GET(
      new Request("https://olex.test/portal/dealers/nearby?lat=41&lng=29"),
    );
    expect(res.status).toBe(429);
    expect(res.headers.get("retry-after")).toBe("30");
  });

  it("answers 502 when Go is unreachable", async () => {
    upstream.fetchUpstream.mockRejectedValue(new Error("down"));
    const res = await GET(
      new Request("https://olex.test/portal/dealers/nearby?lat=41&lng=29"),
    );
    expect(res.status).toBe(502);
  });
});
