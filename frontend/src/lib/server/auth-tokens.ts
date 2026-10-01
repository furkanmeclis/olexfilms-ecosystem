import { decode, encode } from "next-auth/jwt";
import { cookies } from "next/headers";

import { authConfig } from "@/config/auth";

const SESSION_COOKIE_BASES = [
  "authjs.session-token",
  "__Secure-authjs.session-token",
] as const;

const SESSION_COOKIE_CHUNKS = 12;

/**
 * Auth.js picks `__Secure-` cookies from the site URL scheme, not NODE_ENV.
 * Follow the same rule so a production build served over plain http (local
 * prod compose, LAN preview) and an https dev tunnel both read and write the
 * cookie Auth.js actually set.
 */
export function secureCookies() {
  const url = process.env.AUTH_URL ?? process.env.NEXTAUTH_URL;
  if (url) return url.startsWith("https://");
  return process.env.NODE_ENV === "production";
}

export function sessionCookieName() {
  return secureCookies()
    ? "__Secure-authjs.session-token"
    : "authjs.session-token";
}

function sessionCookieOptions() {
  return {
    httpOnly: true,
    secure: secureCookies(),
    sameSite: "lax" as const,
    path: "/",
  };
}

/** Expire the Auth.js session cookie and any chunked variants Auth.js may create. */
async function expireSessionCookies() {
  const jar = await cookies();
  const opts = { ...sessionCookieOptions(), maxAge: 0 };
  for (const base of SESSION_COOKIE_BASES) {
    jar.set(base, "", opts);
    for (let i = 0; i < SESSION_COOKIE_CHUNKS; i += 1) {
      jar.set(`${base}.${i}`, "", opts);
    }
  }
}

async function readSessionToken() {
  const jar = await cookies();
  const name = sessionCookieName();
  const raw = jar.get(name)?.value;
  if (!raw || !process.env.AUTH_SECRET) {
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
      secret: process.env.AUTH_SECRET!,
      salt: name,
    });
  }
  return decode({
    token: raw,
    secret: process.env.AUTH_SECRET,
    salt: name,
  });
}

/** Read the Go access JWT `oid` claim without verifying (cookie already trusted). */
export function organizationUuidFromAccessToken(
  accessToken: string | null | undefined,
): string | null {
  if (!accessToken) return null;
  try {
    const parts = accessToken.split(".");
    if (parts.length < 2 || !parts[1]) return null;
    const json = Buffer.from(parts[1], "base64url").toString("utf8");
    const payload = JSON.parse(json) as { oid?: unknown };
    return typeof payload.oid === "string" && payload.oid.length > 0
      ? payload.oid
      : null;
  } catch {
    return null;
  }
}

export async function getApiTokens() {
  const token = await readSessionToken();
  if (!token) {
    return {
      accessToken: null as string | null,
      refreshToken: null as string | null,
      userId: null as string | null,
      email: null as string | null,
      impersonatorUuid: null as string | null,
      organizationUuid: null as string | null,
    };
  }
  const accessToken = (token.accessToken as string | undefined) ?? null;
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

export async function persistApiTokens(input: {
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
  impersonatorUuid?: string | null;
  expiresIn?: number;
  refreshExpiresAt?: string | null;
}) {
  if (!process.env.AUTH_SECRET) {
    throw new Error("AUTH_SECRET is not configured");
  }

  const existing = await readSessionToken();
  const maxAge = resolveSessionMaxAge(input);
  const organizationUuid = organizationUuidFromAccessToken(input.accessToken);
  const name = sessionCookieName();
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

  // Drop any leftover Auth.js chunks before writing a single cookie.
  await expireSessionCookies();

  const jar = await cookies();
  jar.set(name, payload, {
    ...sessionCookieOptions(),
    maxAge,
  });
}

export async function clearAuthSessionCookie() {
  await expireSessionCookies();
}
