import { beforeEach, describe, expect, it, vi } from "vitest";

type Call = { path: string; method: string; headers: Headers };
const calls: Call[] = [];

vi.mock("@/lib/server/upstream", () => ({
  clientIpFromHeaders: (h: Headers) => h.get("x-real-ip"),
  forwardedHostFromHeaders: (h: Headers) => h.get("host"),
  fetchUpstream: async (
    path: string,
    init: { method: string; headers?: HeadersInit },
  ) => {
    calls.push({
      path,
      method: init.method,
      headers: new Headers(init.headers),
    });
    const body = JSON.stringify({
      success: true,
      data: { access_token: "a", refresh_token: "r" },
    });
    return {
      status: 200,
      headers: new Headers({
        "Content-Type": "application/json",
        "X-Mobile-Api-Version": "1",
        "Set-Cookie": "leak=1",
      }),
      body: new TextEncoder().encode(body).buffer as ArrayBuffer,
    };
  },
}));

// The passthrough must never touch the session cookie helpers.
const sessionTouched = vi.fn();
vi.mock("@/lib/server/auth-tokens", () => ({
  getApiTokens: sessionTouched,
  persistApiTokens: sessionTouched,
  clearAuthSessionCookie: sessionTouched,
}));

import { proxyMobileToUpstream } from "./mobile-proxy";

function req(
  path: string,
  init: RequestInit & { headers?: Record<string, string> } = {},
) {
  return new Request(`http://olexfilms.app/api/v1/mobile/${path}`, init);
}

describe("mobile Bearer passthrough (TEC-91)", () => {
  beforeEach(() => {
    calls.length = 0;
    sessionTouched.mockClear();
  });

  it("forwards a Bearer request without cookies as is (tokens not stripped)", async () => {
    const res = await proxyMobileToUpstream(
      ["auth", "refresh"],
      req("auth/refresh", {
        method: "POST",
        body: JSON.stringify({ refresh_token: "r0" }),
        headers: {
          Authorization: "Bearer mobile-access",
          "Content-Type": "application/json",
          "X-Mobile-Api-Version": "1",
          "X-App-Version": "2.4.0",
          "Idempotency-Key": "k1",
          "X-Real-IP": "203.0.113.9",
          Origin: "https://evil.example",
          "Sec-Fetch-Site": "cross-site",
        },
      }),
    );
    expect(res.status).toBe(200);
    expect(calls).toHaveLength(1);
    const call = calls[0]!;
    expect(call.path).toBe("mobile/auth/refresh");
    expect(call.headers.get("authorization")).toBe("Bearer mobile-access");
    expect(call.headers.get("x-mobile-api-version")).toBe("1");
    expect(call.headers.get("x-app-version")).toBe("2.4.0");
    expect(call.headers.get("idempotency-key")).toBe("k1");
    expect(call.headers.get("x-forwarded-for")).toBe("203.0.113.9");
    expect(call.headers.get("origin")).toBeNull();
    const body = (await res.json()) as { data: { access_token?: string } };
    expect(body.data.access_token).toBe("a");
    expect(res.headers.get("x-mobile-api-version")).toBe("1");
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(sessionTouched).not.toHaveBeenCalled();
  });

  it("refuses a request that carries cookies with 400", async () => {
    const res = await proxyMobileToUpstream(
      ["auth", "me"],
      req("auth/me", {
        headers: {
          Authorization: "Bearer mobile-access",
          Cookie: "panel-session=abc",
        },
      }),
    );
    expect(res.status).toBe(400);
    const body = (await res.json()) as { error: { code: string } };
    expect(body.error.code).toBe("COOKIE_NOT_ALLOWED");
    expect(calls).toHaveLength(0);
    expect(sessionTouched).not.toHaveBeenCalled();
  });

  it("keeps the query string and rejects unsafe segments", async () => {
    await proxyMobileToUpstream(["auth", "me"], req("auth/me?x=1"));
    expect(calls[0]?.path).toBe("mobile/auth/me?x=1");
    for (const segs of [[], [".."], ["auth", "%2e%2e"], ["a/b"]]) {
      const res = await proxyMobileToUpstream(segs, req("x"));
      expect(res.status, segs.join("/")).toBe(404);
    }
    expect(calls).toHaveLength(1);
  });
});
