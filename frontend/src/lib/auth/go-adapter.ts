import type {
  Adapter,
  AdapterAccount,
  AdapterAuthenticator,
  AdapterUser,
} from "next-auth/adapters";

import {
  adapterCreateAuthenticator,
  adapterCreateOAuthUser,
  adapterGetAuthenticator,
  adapterGetUserByEmail,
  adapterGetUserByUUID,
  adapterLinkOAuthAccount,
  adapterGetOAuthAccountUser,
  adapterListAuthenticators,
  adapterUnlinkOAuthAccount,
  adapterUpdateAuthenticatorCounter,
  type AdapterAuthenticatorPayload,
  type AdapterUserPayload,
} from "@/lib/auth/go-adapter-client";

type GoAdapterOptions = {
  pendingGitHubLogins?: Map<string, string>;
};

const OAUTH_PROVIDERS = new Set(["github", "google", "facebook", "apple"]);

function mapUser(user: AdapterUserPayload): AdapterUser {
  return {
    id: user.id,
    email: user.email,
    emailVerified: user.emailVerified ? new Date(user.emailVerified) : null,
    name: user.name ?? null,
  };
}

function mapAuthenticator(
  row: AdapterAuthenticatorPayload,
): AdapterAuthenticator {
  return {
    credentialID: row.credentialID,
    providerAccountId: row.providerAccountId,
    userId: row.userId,
    credentialPublicKey: row.credentialPublicKey,
    counter: row.counter,
    credentialDeviceType: row.credentialDeviceType,
    credentialBackedUp: row.credentialBackedUp,
    transports: row.transports ?? undefined,
  };
}

async function findAuthenticator(credentialID: string) {
  try {
    return await adapterGetAuthenticator(credentialID);
  } catch {
    return null;
  }
}

function isNotFound(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const candidate = error as { code?: string; status?: number };
  return candidate.code === "NOT_FOUND" || candidate.status === 404;
}

function toWebAuthnAccount(
  authenticator: AdapterAuthenticatorPayload,
  provider: string,
): AdapterAccount {
  return {
    userId: authenticator.userId,
    provider,
    providerAccountId: authenticator.providerAccountId,
    type: "webauthn",
  };
}

function serializeTransports(
  transports: AdapterAuthenticator["transports"],
): string | null {
  if (transports == null) return null;
  if (typeof transports === "string") return transports;
  if (Array.isArray(transports)) {
    return (transports as ReadonlyArray<string>).join(",");
  }
  return null;
}

function splitName(fullName: string | null | undefined): {
  name: string;
  surname: string;
} {
  const trimmed = (fullName ?? "").trim();
  if (!trimmed) return { name: "User", surname: "Account" };
  const parts = trimmed.split(/\s+/);
  if (parts.length === 1) return { name: parts[0], surname: "Account" };
  return { name: parts[0], surname: parts.slice(1).join(" ") };
}

export function goAdapter(options: GoAdapterOptions = {}): Adapter {
  const pendingGitHubLogins = options.pendingGitHubLogins;

  return {
    async createUser(user) {
      const email = user.email?.trim();
      if (!email) {
        throw new Error("OAuth registration requires an email");
      }
      const { name, surname } = splitName(user.name);
      const created = await adapterCreateOAuthUser({
        email,
        name,
        surname,
        email_verified: Boolean(user.emailVerified),
      });
      return mapUser(created);
    },
    async getUser(id) {
      try {
        const user = await adapterGetUserByUUID(id);
        return mapUser(user);
      } catch (error) {
        if (isNotFound(error)) return null;
        throw error;
      }
    },
    async getUserByEmail(email) {
      try {
        const user = await adapterGetUserByEmail(email);
        return mapUser(user);
      } catch (error) {
        if (isNotFound(error)) return null;
        throw error;
      }
    },
    async getUserByAccount({ providerAccountId, provider }) {
      if (OAUTH_PROVIDERS.has(provider)) {
        try {
          const user = await adapterGetOAuthAccountUser(
            provider,
            providerAccountId,
          );
          return mapUser(user);
        } catch (error) {
          if (isNotFound(error)) return null;
          throw error;
        }
      }

      const authenticator = await findAuthenticator(providerAccountId);
      if (!authenticator) return null;
      const user = await adapterGetUserByUUID(authenticator.userId);
      return mapUser(user);
    },
    async updateUser() {
      throw new Error("User updates are managed by the Go API");
    },
    async deleteUser() {
      throw new Error("User deletion is managed by the Go API");
    },
    async linkAccount(account) {
      if (OAUTH_PROVIDERS.has(account.provider)) {
        const extended = account as AdapterAccount & {
          access_token?: string | null;
          refresh_token?: string | null;
          expires_at?: number | null;
          token_type?: string | null;
          scope?: string | null;
        };
        const githubLogin =
          account.provider === "github"
            ? pendingGitHubLogins?.get(account.providerAccountId)
            : undefined;
        pendingGitHubLogins?.delete(account.providerAccountId);
        await adapterLinkOAuthAccount({
          userId: account.userId,
          provider: account.provider,
          providerAccountId: account.providerAccountId,
          type: account.type,
          access_token: extended.access_token ?? undefined,
          refresh_token: extended.refresh_token ?? undefined,
          expires_at: extended.expires_at ?? undefined,
          token_type: extended.token_type ?? undefined,
          scope: extended.scope ?? undefined,
          github_login: githubLogin,
        });
      }
      return account;
    },
    async unlinkAccount(account) {
      if (OAUTH_PROVIDERS.has(account.provider)) {
        await adapterUnlinkOAuthAccount(
          account.provider,
          account.providerAccountId,
        );
      }
    },
    async getAccount(providerAccountId, provider) {
      if (OAUTH_PROVIDERS.has(provider)) {
        try {
          const user = await adapterGetOAuthAccountUser(
            provider,
            providerAccountId,
          );
          return {
            userId: user.id,
            provider,
            providerAccountId,
            type: "oauth",
          };
        } catch (error) {
          if (isNotFound(error)) return null;
          throw error;
        }
      }

      const authenticator = await findAuthenticator(providerAccountId);
      if (!authenticator) return null;
      return toWebAuthnAccount(authenticator, provider);
    },
    async createVerificationToken() {
      throw new Error("Verification tokens are managed by the Go API");
    },
    async useVerificationToken() {
      return null;
    },
    async createAuthenticator(authenticator) {
      const created = await adapterCreateAuthenticator({
        userId: authenticator.userId,
        credentialID: authenticator.credentialID,
        providerAccountId: authenticator.providerAccountId,
        credentialPublicKey: authenticator.credentialPublicKey,
        counter: authenticator.counter,
        credentialDeviceType: authenticator.credentialDeviceType,
        credentialBackedUp: authenticator.credentialBackedUp,
        transports: serializeTransports(authenticator.transports),
      });
      return mapAuthenticator(created);
    },
    async getAuthenticator(credentialID) {
      const row = await findAuthenticator(credentialID);
      return row ? mapAuthenticator(row) : null;
    },
    async listAuthenticatorsByUserId(userId) {
      const rows = await adapterListAuthenticators(userId);
      return rows.map(mapAuthenticator);
    },
    async updateAuthenticatorCounter(credentialID, counter) {
      await adapterUpdateAuthenticatorCounter(credentialID, counter);
      const row = await adapterGetAuthenticator(credentialID);
      return mapAuthenticator(row);
    },
  };
}
