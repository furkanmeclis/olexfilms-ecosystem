import NextAuth, { CredentialsSignin } from "next-auth";
import Credentials from "next-auth/providers/credentials";

import { routes } from "@/config/routes";
import {
  InvalidMFACodeError,
  InvalidOTPCodeError,
  MFARequiredError,
  NoPortalAccessError,
  OTPLockedError,
} from "@/lib/auth/credentials-errors";
import {
  loginWithPassword,
  verifyPhoneOTP,
  type GoTokensPayload,
} from "@/lib/auth/go-adapter-client";
import {
  AUTH_BASE_PATHS,
  authCookies,
  secureCookies,
} from "@/lib/server/auth-cookies";
import {
  clientIpFromHeaders,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";

/**
 * Customer / fleet portal Auth.js instance (TEC-90). Separate from the panel
 * instance (auth.ts): own basePath (/api/portal-auth), own cookies
 * (`portal-session`, `portal.csrf-token`, ...), so panel and portal sessions
 * live side by side in one browser and signing out of one keeps the other.
 *
 * Providers:
 *  - `phone-otp`: WhatsApp code (K11), verified server side → aud=portal.
 *  - `fleet-password`: e-mail + password with realm=portal (fleet accounts).
 */

class RateLimitedError extends CredentialsSignin {
  code = "RATE_LIMITED";
}

type AuthorizedUser = {
  id: string;
  email: string | null;
  accessToken: string;
  refreshToken: string;
  expiresIn?: number;
};

/** Subject (user uuid) of a Go access token; the cookie only trusts it after Go issued it. */
function subjectOf(accessToken: string): string | null {
  try {
    const part = accessToken.split(".")[1];
    if (!part) return null;
    const payload = JSON.parse(
      Buffer.from(part, "base64url").toString("utf8"),
    ) as { sub?: unknown };
    return typeof payload.sub === "string" ? payload.sub : null;
  } catch {
    return null;
  }
}

function toUser(
  tokens: GoTokensPayload,
  email: string | null,
): AuthorizedUser | null {
  const id = subjectOf(tokens.access_token);
  if (!id) return null;
  return {
    id,
    email,
    accessToken: tokens.access_token,
    refreshToken: tokens.refresh_token,
    expiresIn: tokens.expires_in,
  };
}

function requestMeta(request: unknown) {
  if (!(request instanceof Request)) {
    return { clientIp: null, forwardedHost: null };
  }
  return {
    clientIp: clientIpFromHeaders(request.headers),
    forwardedHost: forwardedHostFromHeaders(request.headers),
  };
}

function rethrow(error: unknown): null {
  const code = (error as Error & { code?: string }).code;
  if (code === "INVALID_OTP_CODE") throw new InvalidOTPCodeError();
  if (code === "OTP_LOCKED") throw new OTPLockedError();
  if (code === "NO_PORTAL_ACCESS") throw new NoPortalAccessError();
  if (code === "MFA_REQUIRED") throw new MFARequiredError();
  if (code === "INVALID_MFA_CODE") throw new InvalidMFACodeError();
  if (code === "RATE_LIMITED") throw new RateLimitedError();
  return null;
}

const portalAuth = NextAuth({
  trustHost: process.env.AUTH_TRUST_HOST === "true",
  basePath: AUTH_BASE_PATHS.portal,
  useSecureCookies: secureCookies(),
  cookies: authCookies("portal"),
  session: { strategy: "jwt" },
  pages: {
    signIn: routes.portal.login,
    error: routes.portal.login,
  },
  providers: [
    Credentials({
      id: "phone-otp",
      name: "Phone",
      credentials: {
        phone: { label: "Phone", type: "tel" },
        code: { label: "Code", type: "text" },
        country: { label: "Country", type: "text" },
        locale: { label: "Locale", type: "text" },
      },
      async authorize(credentials, request) {
        const phone = String(credentials?.phone ?? "").trim();
        const code = String(credentials?.code ?? "").trim();
        if (!phone || !code) return null;
        try {
          const tokens = await verifyPhoneOTP({
            phone,
            code,
            country: String(credentials?.country ?? "").trim() || null,
            locale: String(credentials?.locale ?? "").trim() || null,
            ...requestMeta(request),
          });
          return toUser(tokens, null);
        } catch (error) {
          return rethrow(error);
        }
      },
    }),
    Credentials({
      id: "fleet-password",
      name: "Fleet",
      credentials: {
        email: { label: "Email", type: "email" },
        password: { label: "Password", type: "password" },
        totp_code: { label: "Authenticator code", type: "text" },
      },
      async authorize(credentials, request) {
        const email = String(credentials?.email ?? "").trim();
        const password = String(credentials?.password ?? "");
        const totp = String(credentials?.totp_code ?? "").trim();
        if (!email || !password) return null;
        const meta = requestMeta(request);
        try {
          const tokens = await loginWithPassword(
            email,
            password,
            totp || undefined,
            undefined,
            meta.clientIp,
            meta.forwardedHost,
            "portal",
          );
          return toUser(tokens, email);
        } catch (error) {
          return rethrow(error);
        }
      },
    }),
  ],
  callbacks: {
    async jwt({ token, user }) {
      if (user) {
        const extended = user as typeof user & {
          accessToken?: string;
          refreshToken?: string;
          expiresIn?: number;
        };
        token.sub = user.id;
        token.email = user.email ?? undefined;
        token.accessToken = extended.accessToken;
        token.refreshToken = extended.refreshToken;
        token.expiresIn = extended.expiresIn;
      }
      return token;
    },
    async session({ session, token }) {
      if (session.user) session.user.id = token.sub ?? "";
      return session;
    },
  },
});

export const {
  handlers: portalHandlers,
  auth: portalSession,
  signOut: portalSignOut,
} = portalAuth;
