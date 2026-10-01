import { decode, encode } from "next-auth/jwt";
import { cookies } from "next/headers";

import { authConfig } from "@/config/auth";
import {
  baseCookieOptions,
  sessionCookieBases,
  sessionCookieName,
  type AuthRealm,
} from "@/lib/server/auth-cookies";

export {
  AUTH_BASE_PATHS,
  AUTH_REALMS,
  authCookies,
  secureCookies,
  sessionCookieName,
  type AuthRealm,
} from "@/lib/server/auth-cookies";

const SESSION_COOKIE_CHUNKS = 12;

/** Expire a realm's session cookie and any chunked variants Auth.js may create. */
async function expireSessionCookies(realm: AuthRealm) {
  const jar = await cookies();
  const opts = { ...baseCookieOptions(), maxAge: 0 };
  for (const base of sessionCookieBases(realm)) {
    jar.set(base, "", opts);
    for (let i = 0; i < SESSION_COOKIE_CHUNKS; i += 1) {
      jar.set(`${base}.${i}`, "", opts);
    }
  }
}

async function readSessionToken(realm: AuthRealm) {
  const jar = await cookies();
  const name = sessionCookieName(realm);
  const raw = jar.get(name)?.value;
  if (!process.env.AUTH_SECRET) return null;
  if (!raw) {
    // Auth.js may have split a large JWT across numbered chunk cookies.
    const chunks: string[] = [];
    for (let i = 0; i < SESSION_COOKIE_CHUNKS; i += 1) {
      const part = jar.get(`${name}.${i}`)?.value;
      if (!part) break;
      chunks.push(part);
    }
    if (chunks.length === 0) return null;
    return decode({
      token: chunks.join(""),
      secret: process.env.AUTH_SECRET,
      salt: name,
    });
  }
  return decode({
    token: raw,
    secret: process.env.AUTH_SECRET,
    salt: name,
  });
}

function accessTokenPayload(
  accessToken: string | null | undefined,
): Record<string, unknown> | null {
  if (!accessToken) return null;
  try {
    const parts = accessToken.split(".");
    if (parts.length < 2 || !parts[1]) return null;
    const json = Buffer.from(parts[1], "base64url").toString("utf8");
    return JSON.parse(json) as Record<string, unknown>;
  } catch {
    return null;
  }
}

/** Read the Go access JWT `oid` claim without verifying (cookie already trusted). */
export function organizationUuidFromAccessToken(
  accessToken: string | null | undefined,
): string | null {
  const oid = accessTokenPayload(accessToken)?.oid;
  return typeof oid === "string" && oid.length > 0 ? oid : null;
}

/** Realm (JWT `aud`) of a Go access token; tokens without aud are panel. */
export function realmFromAccessToken(
  accessToken: string | null | undefined,
): AuthRealm {
  const aud = accessTokenPayload(accessToken)?.aud;
  const values = Array.isArray(aud) ? aud : [aud];
  return values.includes("portal") ? "portal" : "panel";
}

export type ApiTokens = {
  accessToken: string | null;
  refreshToken: string | null;
  userId: string | null;
  email: string | null;
  impersonatorUuid: string | null;
  organizationUuid: string | null;
};

const EMPTY_TOKENS: ApiTokens = {
  accessToken: null,
  refreshToken: null,
  userId: null,
  email: null,
  impersonatorUuid: null,
  organizationUuid: null,
};

export async function getApiTokens(realm: AuthRealm): Promise<ApiTokens> {
  const token = await readSessionToken(realm);
  if (!token) return { ...EMPTY_TOKENS };
  const accessToken = (token.accessToken as string | undefined) ?? null;
  // Defence in depth: a cookie holding the other realm's pair is ignored.
  if (accessToken && realmFromAccessToken(accessToken) !== realm) {
    return { ...EMPTY_TOKENS };
  }
  return {
    accessToken,
    refreshToken: (token.refreshToken as string | undefined) ?? null,
    userId: (token.sub as string | undefined) ?? null,
    email: (token.email as string | undefined) ?? null,
    impersonatorUuid: (token.impersonatorUuid as string | undefined) ?? null,
    organizationUuid:
      (token.organizationUuid as string | undefined) ??
      organizationUuidFromAccessToken(accessToken),
  };
}

function resolveSessionMaxAge(input: {
  expiresIn?: number;
  refreshExpiresAt?: string | Date | null;
}): number {
  if (input.refreshExpiresAt) {
    const ends =
      typeof input.refreshExpiresAt === "string"
        ? Date.parse(input.refreshExpiresAt)
        : input.refreshExpiresAt.getTime();
    if (!Number.isNaN(ends)) {
      const remainingSec = Math.floor((ends - Date.now()) / 1000);
      if (remainingSec > 60) {
        return Math.min(remainingSec, authConfig.refreshMaxAgeSec);
      }
    }
  }
  // Prefer refresh TTL so idle tabs keep the cookie past access expiry.
  return authConfig.refreshMaxAgeSec;
}

export async function persistApiTokens(
  realm: AuthRealm,
  input: {
    accessToken: string;
    refreshToken: string;
    userId: string;
    email?: string | null;
    impersonatorUuid?: string | null;
    expiresIn?: number;
    refreshExpiresAt?: string | null;
  },
) {
  if (!process.env.AUTH_SECRET) {
    throw new Error("AUTH_SECRET is not configured");
  }
  if (realmFromAccessToken(input.accessToken) !== realm) {
    throw new Error(`refusing to store a non-${realm} token pair`);
  }

  const existing = await readSessionToken(realm);
  const maxAge = resolveSessionMaxAge(input);
  const organizationUuid = organizationUuidFromAccessToken(input.accessToken);
  const name = sessionCookieName(realm);
  const payload = await encode({
    token: {
      ...existing,
      sub: input.userId,
      email: input.email ?? existing?.email,
      accessToken: input.accessToken,
      refreshToken: input.refreshToken,
      impersonatorUuid: input.impersonatorUuid ?? undefined,
      organizationUuid: organizationUuid ?? undefined,
      expiresIn: input.expiresIn,
      refreshExpiresAt: input.refreshExpiresAt ?? undefined,
    },
    secret: process.env.AUTH_SECRET,
    salt: name,
    maxAge,
  });

  // Drop any leftover Auth.js chunks of this realm before writing a single cookie.
  await expireSessionCookies(realm);

  const jar = await cookies();
  jar.set(name, payload, {
    ...baseCookieOptions(),
    maxAge,
  });
}

export async function clearAuthSessionCookie(realm: AuthRealm) {
  await expireSessionCookies(realm);
}
