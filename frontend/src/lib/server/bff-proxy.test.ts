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
const clearAuthSessionCookie = vi.fn(async (_realm: string) => {});
const realmsSeen: string[] = [];

vi.mock("@/lib/server/auth-tokens", () => ({
  getApiTokens: async (realm: string) => {
    realmsSeen.push(realm);
    return {
      accessToken: state.accessToken,
      refreshToken: state.refreshToken,
      userId: state.userId,
      email: "a@example.com",
      impersonatorUuid: null,
      organizationUuid: null,
    };
  },
  persistApiTokens: (
    realm: string,
    input: { accessToken: string; refreshToken: string },
  ) => {
    realmsSeen.push(realm);
    return persistApiTokens(input);
  },
  clearAuthSessionCookie: (realm: string) => clearAuthSessionCookie(realm),
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
  forwardedHostFromHeaders: () => null,
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
    if (path.endsWith("/rate-limited")) {
      const res = json(429, {
        success: false,
        error: { code: "RATE_LIMITED" },
      });
      res.headers.set("Retry-After", "59");
      return res;
    }
    if (auth === "Bearer access-new") return json(200, { success: true });
    if (path.startsWith("portal/") || path === "auth/me") {
      return json(200, { success: true });
    }
    return json(401, { success: false });
  },
}));

import { isRealmPathAllowed, proxyToUpstream } from "./bff-proxy";
import { resetSharedRefresh } from "./refresh-single-flight";

function get(path: string) {
  return proxyToUpstream(
    "panel",
    path.split("/"),
    new Request(`http://localhost/api/v1/${path}`),
  );
}

function portalGet(path: string, method = "GET") {
  return proxyToUpstream(
    "portal",
    path.split("/"),
    new Request(`http://localhost/api/portal/v1/${path}`, {
      method,
      ...(method === "GET" ? {} : { body: "{}" }),
    }),
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
    realmsSeen.length = 0;
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
      "panel",
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

describe("BFF response headers (TEC-142)", () => {
  beforeEach(() => {
    state.accessToken = "access-new";
    state.refreshToken = "refresh-new";
    calls.length = 0;
    resetSharedRefresh();
  });

  it("forwards Retry-After on a panel 429", async () => {
    const res = await get("tenant/items/rate-limited");
    expect(res.status).toBe(429);
    expect(res.headers.get("retry-after")).toBe("59");
  });

  it("forwards Retry-After on a portal 429", async () => {
    const res = await portalGet("portal/otp/rate-limited");
    expect(res.status).toBe(429);
    expect(res.headers.get("retry-after")).toBe("59");
  });
});

describe("BFF realms (TEC-90)", () => {
  beforeEach(() => {
    calls.length = 0;
    realmsSeen.length = 0;
    clearAuthSessionCookie.mockClear();
    resetSharedRefresh();
  });

  it("portal BFF answers 404 outside its allowlist without calling the API", async () => {
    for (const path of [
      "platform/users",
      "tenant/settings",
      "me/organizations",
      "auth/organization-context",
      "auth/impersonation/stop",
      "auth/otp/verify",
      "auth/login",
      "auth/register",
      "notifications",
      "internal/auth/users/x",
    ]) {
      const res = await portalGet(path, "POST");
      expect(res.status, path).toBe(404);
    }
    expect(calls).toHaveLength(0);
  });

  it("portal BFF forwards allowlisted paths with the portal session", async () => {
    const res = await portalGet("portal/consents/pending");
    expect(res.status).toBe(200);
    expect(calls.map((c) => c.path)).toEqual(["portal/consents/pending"]);
    expect(new Set(realmsSeen)).toEqual(new Set(["portal"]));
    for (const path of [
      "auth/me",
      "auth/refresh",
      "auth/logout",
      "auth/otp/request",
      "auth/password/forgot",
      "auth/password/reset",
      "public/brand",
      "portal/consents",
    ]) {
      expect(isRealmPathAllowed("portal", path), path).toBe(true);
    }
  });

  it("panel BFF refuses portal routes and the phone OTP flow", async () => {
    for (const path of [
      "portal/consents/pending",
      "auth/otp/request",
      "auth/otp/verify",
    ]) {
      const res = await get(path);
      expect(res.status, path).toBe(404);
    }
    expect(calls).toHaveLength(0);
    expect(isRealmPathAllowed("panel", "platform/users")).toBe(true);
    expect(isRealmPathAllowed("panel", "auth/organization-context")).toBe(true);
  });

  it("panel and portal BFFs refuse the mobile API and the QR exchange (TEC-91)", async () => {
    for (const path of [
      "mobile/auth/me",
      "mobile/auth/login",
      "mobile/push-token",
      "auth/qr/complete",
    ]) {
      const res = await get(path);
      expect(res.status, path).toBe(404);
      const portal = await portalGet(path, "POST");
      expect(portal.status, path).toBe(404);
    }
    expect(calls).toHaveLength(0);
    expect(isRealmPathAllowed("panel", "auth/qr/start")).toBe(true);
    expect(isRealmPathAllowed("panel", "auth/qr/abc/status")).toBe(true);
  });

  it("portal logout clears only the portal cookie", async () => {
    await portalGet("auth/logout", "POST");
    expect(clearAuthSessionCookie).toHaveBeenCalledTimes(1);
    expect(clearAuthSessionCookie).toHaveBeenCalledWith("portal");
  });

  it("panel logout clears only the panel cookie", async () => {
    await proxyToUpstream(
      "panel",
      ["auth", "logout"],
      new Request("http://localhost/api/v1/auth/logout", {
        method: "POST",
        body: "{}",
      }),
    );
    expect(clearAuthSessionCookie).toHaveBeenCalledWith("panel");
    expect(clearAuthSessionCookie).not.toHaveBeenCalledWith("portal");
  });
});
