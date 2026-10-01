import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Set = { name: string; value: string; opts: Record<string, unknown> };
const sets: Set[] = [];
const jar = new Map<string, string>();

vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (name: string) =>
      jar.has(name) ? { name, value: jar.get(name)! } : undefined,
    set: (name: string, value: string, opts: Record<string, unknown>) => {
      sets.push({ name, value, opts });
      if (opts.maxAge === 0) jar.delete(name);
      else jar.set(name, value);
    },
  }),
}));

// The encoded cookie is "salt|json": decode only succeeds with the same salt,
// like Auth.js (salt = cookie name).
vi.mock("next-auth/jwt", () => ({
  encode: async ({ token, salt }: { token: unknown; salt: string }) =>
    `${salt}|${JSON.stringify(token)}`,
  decode: async ({ token, salt }: { token: string; salt: string }) => {
    const [s, ...rest] = token.split("|");
    return s === salt ? JSON.parse(rest.join("|")) : null;
  },
}));

import {
  authCookies,
  clearAuthSessionCookie,
  getApiTokens,
  persistApiTokens,
  realmFromAccessToken,
  secureCookies,
  sessionCookieName,
  type AuthRealm,
} from "./auth-tokens";

function accessToken(aud: AuthRealm | null, sub = "user-1") {
  const payload = Buffer.from(
    JSON.stringify(aud ? { sub, aud: [aud] } : { sub }),
  ).toString("base64url");
  return `h.${payload}.s`;
}

async function signIn(realm: AuthRealm, sub = "user-1") {
  await persistApiTokens(realm, {
    accessToken: accessToken(realm, sub),
    refreshToken: `refresh-${realm}`,
    userId: sub,
  });
}

describe("auth cookies", () => {
  beforeEach(() => {
    sets.length = 0;
    jar.clear();
    vi.stubEnv("AUTH_SECRET", "test-secret");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("uses __Secure- cookies only when AUTH_URL is https", () => {
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("AUTH_URL", "http://localhost:3000");
    expect(secureCookies()).toBe(false);
    expect(sessionCookieName("panel")).toBe("panel-session");
    expect(sessionCookieName("portal")).toBe("portal-session");

    vi.stubEnv("NODE_ENV", "development");
    vi.stubEnv("AUTH_URL", "https://olexfilms.app");
    expect(secureCookies()).toBe(true);
    expect(sessionCookieName("panel")).toBe("__Secure-panel-session");
    expect(sessionCookieName("portal")).toBe("__Secure-portal-session");
  });

  it("falls back to NEXTAUTH_URL, then NODE_ENV", () => {
    vi.stubEnv("AUTH_URL", undefined);
    vi.stubEnv("NEXTAUTH_URL", "https://olexfilms.app");
    expect(secureCookies()).toBe(true);

    vi.stubEnv("NEXTAUTH_URL", undefined);
    vi.stubEnv("NODE_ENV", "production");
    expect(secureCookies()).toBe(true);
  });

  it("prefixes every Auth.js cookie with the realm (no shared names)", () => {
    vi.stubEnv("AUTH_URL", "https://olexfilms.app");
    const panel = Object.values(authCookies("panel")).map((c) => c.name);
    const portal = Object.values(authCookies("portal")).map((c) => c.name);
    expect(panel).toHaveLength(7);
    for (const name of panel) {
      expect(name.startsWith("__Secure-panel")).toBe(true);
    }
    for (const name of portal) {
      expect(name.startsWith("__Secure-portal")).toBe(true);
    }
    expect(panel.filter((n) => portal.includes(n))).toEqual([]);
    expect(authCookies("portal").csrfToken.name).toBe(
      "__Secure-portal.csrf-token",
    );
  });

  it("ties the session cookie lifetime to refresh_expires_at", async () => {
    vi.stubEnv("AUTH_URL", "https://olexfilms.app");
    await persistApiTokens("panel", {
      accessToken: accessToken("panel"),
      refreshToken: "r",
      userId: "u",
      refreshExpiresAt: new Date(Date.now() + 2 * 3600 * 1000).toISOString(),
    });

    const written = sets.find((c) => c.value.length > 0);
    expect(written?.name).toBe("__Secure-panel-session");
    expect(written?.opts.secure).toBe(true);
    const maxAge = written?.opts.maxAge as number;
    expect(maxAge).toBeGreaterThan(2 * 3600 - 10);
    expect(maxAge).toBeLessThanOrEqual(2 * 3600);
  });

  it("keeps panel and portal sessions side by side", async () => {
    vi.stubEnv("AUTH_URL", "http://localhost:3000");
    await signIn("panel", "staff-1");
    await signIn("portal", "customer-1");

    expect((await getApiTokens("panel")).userId).toBe("staff-1");
    expect((await getApiTokens("panel")).refreshToken).toBe("refresh-panel");
    expect((await getApiTokens("portal")).userId).toBe("customer-1");
    expect((await getApiTokens("portal")).refreshToken).toBe("refresh-portal");
  });

  it("signing out of one realm keeps the other session", async () => {
    vi.stubEnv("AUTH_URL", "http://localhost:3000");
    await signIn("panel");
    await signIn("portal");

    sets.length = 0;
    await clearAuthSessionCookie("panel");
    expect(sets.every((c) => c.name.includes("panel-session"))).toBe(true);
    expect((await getApiTokens("panel")).accessToken).toBeNull();
    expect((await getApiTokens("portal")).accessToken).not.toBeNull();

    await signIn("panel");
    sets.length = 0;
    await clearAuthSessionCookie("portal");
    expect(sets.every((c) => c.name.includes("portal-session"))).toBe(true);
    expect((await getApiTokens("portal")).accessToken).toBeNull();
    expect((await getApiTokens("panel")).accessToken).not.toBeNull();
  });

  it("refreshing one realm does not rewrite the other cookie", async () => {
    vi.stubEnv("AUTH_URL", "http://localhost:3000");
    await signIn("panel");
    sets.length = 0;
    await signIn("portal");
    expect(sets.some((c) => c.name.includes("panel"))).toBe(false);
  });

  it("refuses to store a token pair of the other realm", async () => {
    await expect(
      persistApiTokens("panel", {
        accessToken: accessToken("portal"),
        refreshToken: "r",
        userId: "u",
      }),
    ).rejects.toThrow();
    await expect(
      persistApiTokens("portal", {
        accessToken: accessToken("panel"),
        refreshToken: "r",
        userId: "u",
      }),
    ).rejects.toThrow();
  });

  it("ignores a cookie holding the other realm's token", async () => {
    vi.stubEnv("AUTH_URL", "http://localhost:3000");
    // A tampered / stale panel cookie with a portal token inside.
    jar.set(
      "panel-session",
      `panel-session|${JSON.stringify({ sub: "u", accessToken: accessToken("portal") })}`,
    );
    expect((await getApiTokens("panel")).accessToken).toBeNull();
  });

  it("reads aud from the Go access token (no aud = panel)", () => {
    expect(realmFromAccessToken(accessToken("portal"))).toBe("portal");
    expect(realmFromAccessToken(accessToken("panel"))).toBe("panel");
    expect(realmFromAccessToken(accessToken(null))).toBe("panel");
    expect(realmFromAccessToken("garbage")).toBe("panel");
  });
});
