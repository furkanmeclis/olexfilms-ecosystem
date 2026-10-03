import { beforeEach, describe, expect, it, vi } from "vitest";

const upstream = vi.hoisted(() => ({ fetch: vi.fn(), down: false }));
vi.mock("@/lib/server/upstream", () => ({
  // An unreachable backend rejects outside the spy.
  fetchUpstream: (...args: unknown[]) =>
    upstream.down
      ? Promise.reject(new TypeError("fetch failed"))
      : upstream.fetch(...args),
  clientIpFromHeaders: (h: Headers) => h.get("x-real-ip"),
  forwardedHostFromHeaders: (h: Headers) => h.get("host"),
}));

import { GET } from "./route";

const TOKEN = "aZ3kP9qXbT";

function call(token: string) {
  return GET(
    new Request(`http://localhost/s/${token}`, {
      headers: { "x-real-ip": "203.0.113.5", host: "olexfilms.app" },
    }),
    { params: Promise.resolve({ token }) },
  );
}

function reply(status: number, data?: unknown) {
  const body = new TextEncoder().encode(
    JSON.stringify(
      data === undefined ? { success: false } : { success: true, data },
    ),
  ).buffer;
  upstream.fetch.mockResolvedValue({ status, headers: new Headers(), body });
}

beforeEach(() => {
  upstream.fetch.mockReset();
  upstream.down = false;
});

describe("GET /s/[token]", () => {
  it("302s to the internal target and forwards IP and host", async () => {
    reply(200, {
      token: TOKEN,
      target_path: "/garanti/AbCdEfGh?lang=tr",
      expires_at: null,
    });
    const res = await call(TOKEN);
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toBe("/garanti/AbCdEfGh?lang=tr");
    expect(res.headers.get("cache-control")).toBe("no-store");
    const [path, init] = upstream.fetch.mock.calls[0]!;
    expect(path).toBe(`public/short-urls/${TOKEN}`);
    const h = new Headers(init.headers);
    expect(h.get("x-forwarded-for")).toBe("203.0.113.5");
    expect(h.get("x-forwarded-host")).toBe("olexfilms.app");
  });

  it("resolves an old hub token (8 characters)", async () => {
    reply(200, { token: "aZ3kP9qX", target_path: "/portal", expires_at: null });
    const res = await call("aZ3kP9qX");
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toBe("/portal");
  });

  it("sends expired, unknown and failed links to the notice page", async () => {
    reply(410);
    expect((await call(TOKEN)).headers.get("location")).toBe(
      "/link-unavailable?reason=expired",
    );
    reply(404);
    expect((await call(TOKEN)).headers.get("location")).toBe(
      "/link-unavailable?reason=not_found",
    );
    reply(429);
    expect((await call(TOKEN)).headers.get("location")).toBe(
      "/link-unavailable?reason=error",
    );
    upstream.down = true;
    expect((await call(TOKEN)).headers.get("location")).toBe(
      "/link-unavailable?reason=error",
    );
  });

  it("rejects a malformed token without a request", async () => {
    const res = await call("bad%2Ftoken");
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toBe(
      "/link-unavailable?reason=not_found",
    );
    expect(upstream.fetch).not.toHaveBeenCalled();
  });

  it("never redirects to another origin", async () => {
    for (const target of [
      "https://evil.example/portal",
      "//evil.example/portal",
      "/\\evil.example",
      "/platform/users",
      "/portal/../platform",
    ]) {
      reply(200, { token: TOKEN, target_path: target, expires_at: null });
      expect((await call(TOKEN)).headers.get("location")).toBe(
        "/link-unavailable?reason=error",
      );
    }
  });
});
