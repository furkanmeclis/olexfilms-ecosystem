import NextAuth from "next-auth";
import type { Provider } from "next-auth/providers";
import Apple from "next-auth/providers/apple";
import Credentials from "next-auth/providers/credentials";
import Facebook from "next-auth/providers/facebook";
import GitHub from "next-auth/providers/github";
import Google from "next-auth/providers/google";
import Passkey from "next-auth/providers/passkey";
import { NextResponse } from "next/server";

import { routes } from "@/config/routes";
import {
  InvalidMFACodeError,
  MFANotEnrolledError,
  MFARequiredError,
  NoTenantMembershipError,
  OrganizationAccessExpiredError,
} from "@/lib/auth/credentials-errors";
import { goAdapter } from "@/lib/auth/go-adapter";
import {
  adapterGetOAuthConfig,
  adapterGetOAuthAccountUser,
  adapterGetUserByEmail,
  adapterIssueSession,
  loginWithPassword,
  type OAuthConfigPayload,
} from "@/lib/auth/go-adapter-client";
import {
  clientIpFromHeaders,
  forwardedHostFromHeaders,
} from "@/lib/server/upstream";
import { fetchAppPublicConfig } from "@/services/app-config.service";

const pendingGitHubLogins = new Map<string, string>();

const OAUTH_PROVIDERS = ["github", "google", "facebook", "apple"] as const;
type OAuthProviderId = (typeof OAUTH_PROVIDERS)[number];

const GUEST_AUTH_PREFIXES = [
  routes.guest.login,
  routes.guest.register,
  routes.guest.forgotPassword,
  routes.guest.resetPassword,
  routes.guest.verifyEmail,
];

function isTenantGuestPath(pathname: string) {
  return /^\/t\/[^/]+\/login\/?$/.test(pathname);
}

function isTenantPath(pathname: string) {
  return /^\/t\/[^/]+(\/.*)?$/.test(pathname);
}

function isGuestAuthPath(pathname: string) {
  return GUEST_AUTH_PREFIXES.some(
    (path) => pathname === path || pathname.startsWith(`${path}/`),
  );
}

function isProtectedPath(pathname: string) {
  if (pathname === routes.public.root) return false;
  if (isTenantGuestPath(pathname)) return false;
  if (isTenantPath(pathname)) return true;
  if (pathname.startsWith(`${routes.cms.profile.root}`)) return true;
  if (pathname === routes.platform.root) return true;
  if (
    pathname.startsWith(`${routes.platform.root}/`) &&
    !isGuestAuthPath(pathname)
  ) {
    return true;
  }
  return false;
}

function isOAuthProvider(
  provider: string | undefined,
): provider is OAuthProviderId {
  return (
    provider !== undefined &&
    (OAUTH_PROVIDERS as readonly string[]).includes(provider)
  );
}

// Providers are built by making several backend calls. Cache the result for
// PROVIDERS_CACHE_TTL ms so repeated session/CSRF requests don't re-issue all
// those HTTP round-trips on every hit.
const PROVIDERS_CACHE_TTL = 5 * 60 * 1000; // 5 minutes
let _providersCache: { providers: Provider[]; builtAt: number } | null = null;
let _providersBuildPromise: Promise<Provider[]> | null = null;

async function loadOAuthConfig(
  provider: OAuthProviderId,
): Promise<OAuthConfigPayload | null> {
  try {
    const config = await adapterGetOAuthConfig(provider);
    if (config.enabled && config.client_id && config.client_secret) {
      return config;
    }
  } catch {
    // Provider stays disabled when config is unavailable.
  }
  return null;
}

async function _doBuildProviders(): Promise<Provider[]> {
  let passwordLogin = true;
  let passkeyLogin = true;
  try {
    const appConfig = await fetchAppPublicConfig("en");
    passwordLogin = appConfig.auth_methods.password.login;
    passkeyLogin = appConfig.auth_methods.passkey.login;
  } catch {
    // Keep defaults when public config is unavailable.
  }

  // Fetch all OAuth configs in parallel instead of sequentially.
  const [github, google, facebook, apple] = await Promise.all([
    loadOAuthConfig("github"),
    loadOAuthConfig("google"),
    loadOAuthConfig("facebook"),
    loadOAuthConfig("apple"),
  ]);

  const providers: Provider[] = [];

  if (passwordLogin) {
    providers.push(
      Credentials({
        credentials: {
          email: { label: "Email", type: "email" },
          password: { label: "Password", type: "password" },
          totp_code: { label: "Authenticator code", type: "text" },
          organization_slug: { label: "Organization slug", type: "text" },
        },
        async authorize(credentials, request) {
          const email = String(credentials?.email ?? "").trim();
          const password = String(credentials?.password ?? "");
          const totpCode = String(credentials?.totp_code ?? "").trim();
          const organizationSlug = String(
            credentials?.organization_slug ?? "",
          ).trim();
          if (!email || !password) {
            console.error("[auth][credentials] missing email/password", {
              hasEmail: Boolean(email),
              hasPassword: Boolean(password),
            });
            return null;
          }

          try {
            const tokens = await loginWithPassword(
              email,
              password,
              totpCode || undefined,
              organizationSlug || undefined,
              request instanceof Request
                ? clientIpFromHeaders(request.headers)
                : null,
              request instanceof Request
                ? forwardedHostFromHeaders(request.headers)
                : null,
            );
            const user = await adapterGetUserByEmail(email);
            return {
              id: user.id,
              email: user.email,
              name: user.name,
              accessToken: tokens.access_token,
              refreshToken: tokens.refresh_token,
              expiresIn: tokens.expires_in,
            };
          } catch (error) {
            const code = (error as Error & { code?: string }).code;
            console.error("[auth][credentials] authorize failed", {
              email,
              code,
              message: error instanceof Error ? error.message : String(error),
              status: (error as Error & { status?: number }).status,
            });
            if (code === "MFA_REQUIRED") {
              throw new MFARequiredError();
            }
            if (code === "INVALID_MFA_CODE") {
              throw new InvalidMFACodeError();
            }
            if (code === "MFA_NOT_ENROLLED") {
              throw new MFANotEnrolledError();
            }
            if (code === "NO_TENANT_MEMBERSHIP") {
              throw new NoTenantMembershipError();
            }
            if (code === "ORGANIZATION_ACCESS_EXPIRED") {
              throw new OrganizationAccessExpiredError();
            }
            return null;
          }
        },
      }),
    );
  }

  if (passkeyLogin) {
    providers.push(Passkey({}));
  }

  if (github) {
    providers.push(
      GitHub({
        clientId: github.client_id,
        clientSecret: github.client_secret,
      }),
    );
  }

  if (google) {
    providers.push(
      Google({
        clientId: google.client_id,
        clientSecret: google.client_secret,
      }),
    );
  }

  if (facebook) {
    providers.push(
      Facebook({
        clientId: facebook.client_id,
        clientSecret: facebook.client_secret,
      }),
    );
  }

  if (apple) {
    providers.push(
      Apple({
        clientId: apple.client_id,
        clientSecret: apple.client_secret,
      }),
    );
  }

  return providers;
}

async function buildProviders(): Promise<Provider[]> {
  const now = Date.now();
  if (_providersCache && now - _providersCache.builtAt < PROVIDERS_CACHE_TTL) {
    return _providersCache.providers;
  }
  // Deduplicate concurrent calls while the first build is in flight.
  if (!_providersBuildPromise) {
    _providersBuildPromise = _doBuildProviders()
      .then((providers) => {
        _providersCache = { providers, builtAt: Date.now() };
        _providersBuildPromise = null;
        return providers;
      })
      .catch((err) => {
        _providersBuildPromise = null;
        throw err;
      });
  }
  return _providersBuildPromise;
}

const authHandlers = NextAuth(async () => ({
  adapter: goAdapter({ pendingGitHubLogins }),
  trustHost: process.env.AUTH_TRUST_HOST === "true",
  session: { strategy: "jwt" },
  providers: await buildProviders(),
  experimental: {
    enableWebAuthn: true,
  },
  pages: {
    signIn: routes.guest.login,
    error: routes.guest.login,
  },
  callbacks: {
    authorized({ auth, request }) {
      const { pathname } = request.nextUrl;
      if (!isProtectedPath(pathname) || auth) {
        return true;
      }

      const url = request.nextUrl.clone();
      url.pathname = routes.guest.login;
      url.searchParams.set("next", pathname);
      return NextResponse.redirect(url);
    },
    async signIn({ account, profile }) {
      if (!isOAuthProvider(account?.provider)) {
        return true;
      }

      const provider = account.provider;

      if (
        provider === "github" &&
        profile &&
        typeof profile === "object" &&
        "login" in profile &&
        typeof profile.login === "string" &&
        account.providerAccountId
      ) {
        pendingGitHubLogins.set(account.providerAccountId, profile.login);
      }

      const providerAccountId = account.providerAccountId ?? "";
      try {
        await adapterGetOAuthAccountUser(provider, providerAccountId);
        return true;
      } catch {
        // Not linked yet.
      }

      const { getApiTokens } = await import("@/lib/server/auth-tokens");
      const session = await getApiTokens();
      if (session.userId) {
        return true;
      }

      try {
        const config = await adapterGetOAuthConfig(provider);
        if (config.register_allowed) {
          return true;
        }
      } catch {
        // fall through
      }

      return `${routes.guest.login}?error=OAuthNotLinked`;
    },
    async jwt({ token, user, account, trigger, session }) {
      if (user) {
        token.sub = user.id;
        token.email = user.email;
        const extended = user as typeof user & {
          accessToken?: string;
          refreshToken?: string;
          expiresIn?: number;
        };
        if (extended.accessToken && extended.refreshToken) {
          token.accessToken = extended.accessToken;
          token.refreshToken = extended.refreshToken;
          token.expiresIn = extended.expiresIn;
          const { organizationUuidFromAccessToken } =
            await import("@/lib/server/auth-tokens");
          const oid = organizationUuidFromAccessToken(extended.accessToken);
          if (oid) token.organizationUuid = oid;
          else delete token.organizationUuid;
        }
      }

      if (
        account &&
        (account.provider === "passkey" || isOAuthProvider(account.provider)) &&
        token.sub &&
        !token.accessToken
      ) {
        try {
          const authMethod =
            account.provider === "passkey" ? "passkey" : "oauth";
          const tokens = await adapterIssueSession(token.sub, authMethod);
          token.accessToken = tokens.access_token;
          token.refreshToken = tokens.refresh_token;
          token.expiresIn = tokens.expires_in;
          delete token.organizationUuid;
        } catch {
          token.error = isOAuthProvider(account.provider)
            ? "OAuthSessionError"
            : "PasskeySessionError";
        }
      }

      if (trigger === "update" && session) {
        const patch = session as {
          accessToken?: string;
          refreshToken?: string;
          expiresIn?: number;
          organizationUuid?: string | null;
        };
        if (patch.accessToken) {
          token.accessToken = patch.accessToken;
          const { organizationUuidFromAccessToken } =
            await import("@/lib/server/auth-tokens");
          const oid = organizationUuidFromAccessToken(patch.accessToken);
          if (oid) token.organizationUuid = oid;
          else delete token.organizationUuid;
        }
        if (patch.refreshToken) token.refreshToken = patch.refreshToken;
        if (patch.expiresIn) token.expiresIn = patch.expiresIn;
        if (patch.organizationUuid !== undefined) {
          if (patch.organizationUuid) {
            token.organizationUuid = patch.organizationUuid;
          } else {
            delete token.organizationUuid;
          }
        }
      }

      return token;
    },
    async session({ session, token }) {
      if (session.user) {
        session.user.id = token.sub ?? "";
      }
      const nextSession = session as typeof session & {
        organizationUuid?: string | null;
        error?: string;
      };
      nextSession.organizationUuid =
        (token.organizationUuid as string | undefined) ?? null;
      nextSession.error = token.error as
        "PasskeySessionError" | "OAuthSessionError" | undefined;
      return nextSession;
    },
  },
}));

export const { handlers, auth, signIn, signOut } = authHandlers;
