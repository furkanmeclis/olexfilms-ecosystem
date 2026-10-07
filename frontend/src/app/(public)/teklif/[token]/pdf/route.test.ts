import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const upstream = vi.hoisted(() => ({ fetch: vi.fn(), down: false }));
vi.mock("@/lib/server/upstream", () => ({
  fetchUpstreamStream: (...args: unknown[]) =>
    upstream.down
      ? Promise.reject(new TypeError("fetch failed"))
      : upstream.fetch(...args),
  clientIpFromHeaders: (h: Headers) => h.get("x-real-ip"),
  forwardedHostFromHeaders: (h: Headers) => h.get("host"),
}));

import { GET } from "./route";

const TOKEN = "3f1c2b7a-8d4e-4b6f-9a1c-2e3d4f5a6b7c";

function call(token = TOKEN, query = "?lang=tr") {
  return GET(
    new Request(`http://localhost/teklif/${token}/pdf${query}`, {
      headers: { "x-real-ip": "203.0.113.5", host: "olexfilms.app" },
    }),
    { params: Promise.resolve({ token }) },
  );
}

function reply(status: number, headers: HeadersInit = {}, body = "%PDF") {
  return {
    status,
    headers: new Headers(headers),
    body: new Response(body).body,
  };
}

beforeEach(() => {
  upstream.fetch.mockReset();
  upstream.down = false;
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

async function run(token?: string, query?: string) {
  const pending = call(token, query);
  await vi.runAllTimersAsync();
  return pending;
}

describe("GET /teklif/[token]/pdf (TEC-320)", () => {
  it("streams the PDF and forwards locale, IP and host", async () => {
    upstream.fetch.mockResolvedValue(
      reply(200, {
        "content-type": "application/pdf",
        "content-disposition": 'attachment; filename="Q-000042.pdf"',
        "set-cookie": "x=1",
      }),
    );
    const res = await run();
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("application/pdf");
    expect(res.headers.get("content-disposition")).toContain("Q-000042.pdf");
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(res.headers.get("x-robots-tag")).toContain("noindex");
    expect(await res.text()).toBe("%PDF");
    const [path, init] = upstream.fetch.mock.calls[0]!;
    expect(path).toBe(`public/quotes/${TOKEN}/pdf/file?locale=tr`);
    const h = new Headers(init.headers);
    expect(h.get("x-forwarded-for")).toBe("203.0.113.5");
    expect(h.get("x-forwarded-host")).toBe("olexfilms.app");
  });

  it("retries a queued render once, then streams it", async () => {
    upstream.fetch
      .mockResolvedValueOnce(reply(202, {}, "{}"))
      .mockResolvedValueOnce(reply(200));
    const res = await run();
    expect(res.status).toBe(200);
    expect(upstream.fetch).toHaveBeenCalledTimes(2);
  });

  it("sends the visitor back with a pending notice when still queued", async () => {
    upstream.fetch.mockResolvedValue(reply(202, {}, "{}"));
    const res = await run();
    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe(
      `/teklif/${TOKEN}?lang=tr&pdf=pending`,
    );
    expect(upstream.fetch).toHaveBeenCalledTimes(2);
  });

  it("maps 404, 429, errors and a down backend back to the page", async () => {
    upstream.fetch.mockResolvedValueOnce(reply(404));
    expect((await run()).headers.get("location")).toBe(
      `/teklif/${TOKEN}?lang=tr`,
    );
    upstream.fetch.mockResolvedValueOnce(reply(429));
    expect((await run()).headers.get("location")).toBe(
      `/teklif/${TOKEN}?lang=tr&pdf=rate_limited`,
    );
    upstream.fetch.mockResolvedValueOnce(reply(500));
    expect((await run()).headers.get("location")).toBe(
      `/teklif/${TOKEN}?lang=tr&pdf=unavailable`,
    );
    upstream.down = true;
    expect((await run()).headers.get("location")).toBe(
      `/teklif/${TOKEN}?lang=tr&pdf=unavailable`,
    );
  });

  it("never calls Go for a malformed token", async () => {
    const res = await run("..%2Fpanel");
    expect(res.status).toBe(303);
    expect(upstream.fetch).not.toHaveBeenCalled();
  });
});
