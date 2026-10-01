import { beforeEach, describe, expect, it, vi } from "vitest";

const state = {
  accessToken: "access-old" as string | null,
  refreshToken: "refresh-old" as string | null,
  userId: "user-1" as string | null,
};

const persistApiTokens = vi.fn(
  async (input: { accessToken: string; refreshToken: string }) => {
    state.accessToken = input.accessToken;
    state.refreshToken = input.refreshToken;
  },
);
const clearAuthSessionCookie = vi.fn(async () => {});

vi.mock("@/lib/server/auth-tokens", () => ({
  getApiTokens: async () => ({
    accessToken: state.accessToken,
    refreshToken: state.refreshToken,
    userId: state.userId,
    email: "a@example.com",
    impersonatorUuid: null,
    organizationUuid: null,
  }),
  persistApiTokens: (input: { accessToken: string; refreshToken: string }) =>
    persistApiTokens(input),
  clearAuthSessionCookie: () => clearAuthSessionCookie(),
}));

type Call = { path: string; auth: string | null };
const calls: Call[] = [];
let refreshStatus = 200;

function json(status: number, body: unknown) {
  return {
    status,
    headers: new Headers({ "Content-Type": "application/json" }),
    body: new TextEncoder().encode(JSON.stringify(body)).buffer as ArrayBuffer,
  };
}

vi.mock("@/lib/server/upstream", () => ({
  clientIpFromHeaders: () => null,
  fetchUpstreamStream: vi.fn(),
  fetchUpstream: async (
    path: string,
    init: { method: string; headers?: HeadersInit },
  ) => {
    const auth = new Headers(init.headers).get("Authorization");
    calls.push({ path, auth });
    if (path === "auth/refresh") {
      // Let every concurrent caller pile up on the same rotation.
      await new Promise((r) => setTimeout(r, 20));
      if (refreshStatus !== 200) return json(refreshStatus, { success: false });
      return json(200, {
        success: true,
        data: {
          access_token: "access-new",
          refresh_token: "refresh-new",
          expires_in: 900,
          refresh_expires_at: new Date(Date.now() + 3_600_000).toISOString(),
        },
      });
    }
    if (auth === "Bearer access-new") return json(200, { success: true });
    return json(401, { success: false });
  },
}));

import { proxyToUpstream } from "./bff-proxy";
import { resetSharedRefresh } from "./refresh-single-flight";

function get(path: string) {
  return proxyToUpstream(
    path.split("/"),
    new Request(`http://localhost/api/v1/${path}`),
  );
}

describe("BFF refresh", () => {
  beforeEach(() => {
    state.accessToken = "access-old";
    state.refreshToken = "refresh-old";
    state.userId = "user-1";
    calls.length = 0;
    refreshStatus = 200;
    persistApiTokens.mockClear();
    clearAuthSessionCookie.mockClear();
    resetSharedRefresh();
  });

  it("shares one refresh across 6 parallel requests (single flight)", async () => {
    const responses = await Promise.all(
      Array.from({ length: 6 }, (_, i) => get(`tenant/items/${i}`)),
    );

    expect(responses.map((r) => r.status)).toEqual([
      200, 200, 200, 200, 200, 200,
    ]);
    expect(calls.filter((c) => c.path === "auth/refresh")).toHaveLength(1);
    expect(clearAuthSessionCookie).not.toHaveBeenCalled();
    // The refresh lifetime from the API reaches the cookie writer.
    const persisted = persistApiTokens.mock.calls[0]?.[0] as {
      refreshExpiresAt?: string | null;
    };
    expect(typeof persisted.refreshExpiresAt).toBe("string");
  });

  it("keeps the session and answers 503 when refresh fails with 5xx", async () => {
    refreshStatus = 502;
    const res = await get("tenant/items/1");

    expect(res.status).toBe(503);
    expect(clearAuthSessionCookie).not.toHaveBeenCalled();
    expect(persistApiTokens).not.toHaveBeenCalled();
    expect(state.refreshToken).toBe("refresh-old");
  });

  it("answers 503 on an explicit refresh while the API is down", async () => {
    refreshStatus = 503;
    const res = await proxyToUpstream(
      ["auth", "refresh"],
      new Request("http://localhost/api/v1/auth/refresh", {
        method: "POST",
        body: "{}",
      }),
    );

    expect(res.status).toBe(503);
    expect(clearAuthSessionCookie).not.toHaveBeenCalled();
  });

  it("passes a rejected refresh (401) through as 401", async () => {
    refreshStatus = 401;
    const res = await get("tenant/items/1");

    expect(res.status).toBe(401);
  });
});
