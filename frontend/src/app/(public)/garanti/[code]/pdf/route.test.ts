import { beforeEach, describe, expect, it, vi } from "vitest";

const upstream = vi.hoisted(() => ({ fetch: vi.fn(), down: false }));
vi.mock("@/lib/server/upstream", () => ({
  // An unreachable backend rejects outside the spy.
  fetchUpstreamStream: (...args: unknown[]) =>
    upstream.down
      ? Promise.reject(new TypeError("fetch failed"))
      : upstream.fetch(...args),
  clientIpFromHeaders: (h: Headers) => h.get("x-real-ip"),
  forwardedHostFromHeaders: (h: Headers) => h.get("host"),
}));

import { GET } from "./route";

const CODE = "AbCdEfGhIjKlMnOpQrSt_-";

function call(code: string, query = "?lang=tr&tz=Europe%2FIstanbul") {
  return GET(
    new Request(`http://localhost/garanti/${code}/pdf${query}`, {
      headers: { "x-real-ip": "203.0.113.5", host: "olexfilms.app" },
    }),
    { params: Promise.resolve({ code }) },
  );
}

function reply(status: number, headers: HeadersInit = {}, body = "%PDF") {
  upstream.fetch.mockResolvedValue({
    status,
    headers: new Headers(headers),
    body: new Response(body).body,
  });
}

beforeEach(() => {
  upstream.fetch.mockReset();
  upstream.down = false;
});

describe("GET /garanti/[code]/pdf", () => {
  it("streams the PDF and forwards lang, tz, IP and host", async () => {
    reply(200, {
      "content-type": "application/pdf",
      "content-disposition": `attachment; filename="warranty-${CODE}.pdf"`,
      "set-cookie": "x=1",
    });
    const res = await call(CODE);
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("application/pdf");
    expect(res.headers.get("content-disposition")).toContain("attachment");
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(await res.text()).toBe("%PDF");
    const [path, init] = upstream.fetch.mock.calls[0]!;
    expect(path).toBe(
      `public/warranties/${CODE}/pdf?lang=tr&tz=Europe%2FIstanbul`,
    );
    const h = new Headers(init.headers);
    expect(h.get("x-forwarded-for")).toBe("203.0.113.5");
    expect(h.get("x-forwarded-host")).toBe("olexfilms.app");
  });

  it("accepts an old hub warranty number", async () => {
    reply(200, { "content-type": "application/pdf" });
    const res = await call("DS7K2M9QX4");
    expect(res.status).toBe(200);
  });

  it("sends a malformed code back to the page without a request", async () => {
    const res = await call("bad%20code");
    expect(res.status).toBe(303);
    expect(upstream.fetch).not.toHaveBeenCalled();
  });

  it("maps 404, 429 and failures to the page", async () => {
    reply(404);
    expect((await call(CODE)).headers.get("location")).toBe(
      `/garanti/${CODE}?lang=tr`,
    );
    reply(429, { "retry-after": "30" });
    expect((await call(CODE)).headers.get("location")).toBe(
      `/garanti/${CODE}?lang=tr&pdf=rate_limited`,
    );
    reply(503);
    expect((await call(CODE)).headers.get("location")).toBe(
      `/garanti/${CODE}?lang=tr&pdf=unavailable`,
    );
  });

  it("maps an unreachable backend to the page", async () => {
    upstream.down = true;
    const res = await call(CODE);
    expect(res.headers.get("location")).toBe(
      `/garanti/${CODE}?lang=tr&pdf=unavailable`,
    );
  });
});
