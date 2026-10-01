import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Set = { name: string; value: string; opts: Record<string, unknown> };
const sets: Set[] = [];

vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: () => undefined,
    set: (name: string, value: string, opts: Record<string, unknown>) => {
      sets.push({ name, value, opts });
    },
  }),
}));

vi.mock("next-auth/jwt", () => ({
  encode: async () => "encoded",
  decode: async () => null,
}));

import {
  persistApiTokens,
  secureCookies,
  sessionCookieName,
} from "./auth-tokens";

describe("auth cookies", () => {
  beforeEach(() => {
    sets.length = 0;
    vi.stubEnv("AUTH_SECRET", "test-secret");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("uses __Secure- cookies only when AUTH_URL is https", () => {
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("AUTH_URL", "http://localhost:3000");
    expect(secureCookies()).toBe(false);
    expect(sessionCookieName()).toBe("authjs.session-token");

    vi.stubEnv("NODE_ENV", "development");
    vi.stubEnv("AUTH_URL", "https://olexfilms.app");
    expect(secureCookies()).toBe(true);
    expect(sessionCookieName()).toBe("__Secure-authjs.session-token");
  });

  it("falls back to NEXTAUTH_URL, then NODE_ENV", () => {
    vi.stubEnv("AUTH_URL", undefined);
    vi.stubEnv("NEXTAUTH_URL", "https://olexfilms.app");
    expect(secureCookies()).toBe(true);

    vi.stubEnv("NEXTAUTH_URL", undefined);
    vi.stubEnv("NODE_ENV", "production");
    expect(secureCookies()).toBe(true);
  });

  it("ties the session cookie lifetime to refresh_expires_at", async () => {
    vi.stubEnv("AUTH_URL", "https://olexfilms.app");
    await persistApiTokens({
      accessToken: "a",
      refreshToken: "r",
      userId: "u",
      refreshExpiresAt: new Date(Date.now() + 2 * 3600 * 1000).toISOString(),
    });

    const written = sets.find((c) => c.value === "encoded");
    expect(written?.name).toBe("__Secure-authjs.session-token");
    expect(written?.opts.secure).toBe(true);
    const maxAge = written?.opts.maxAge as number;
    expect(maxAge).toBeGreaterThan(2 * 3600 - 10);
    expect(maxAge).toBeLessThanOrEqual(2 * 3600);
  });
});
