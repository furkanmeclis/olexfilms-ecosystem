import { fetchUpstream } from "@/lib/server/upstream";

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code?: string; message?: string };
};

function adapterKey(): string {
  const key = process.env.AUTH_ADAPTER_SECRET;
  if (!key) {
    throw new Error("AUTH_ADAPTER_SECRET is not configured");
  }
  return key;
}

function adapterHeaders(): HeadersInit {
  return {
    Accept: "application/json",
    "Content-Type": "application/json",
    "X-Auth-Adapter-Key": adapterKey(),
  };
}

async function unwrap<T>(
  result: Awaited<ReturnType<typeof fetchUpstream>>,
): Promise<T> {
  const text = new TextDecoder().decode(result.body);
  if (result.status >= 400) {
    let message = `Adapter request failed (${result.status})`;
    let code: string | undefined;
    try {
      const env = JSON.parse(text) as Envelope<unknown>;
      message = env.error?.message ?? message;
      code = env.error?.code;
    } catch {
      // ignore
    }
    const error = new Error(message) as Error & {
      code?: string;
      status?: number;
    };
    error.code = code;
    error.status = result.status;
    throw error;
  }
  const env = JSON.parse(text) as Envelope<T>;
  if (!env.success || env.data === undefined) {
    throw new Error("Invalid adapter response envelope");
  }
  return env.data;
}

export type AdapterUserPayload = {
  id: string;
  email: string;
  emailVerified?: string | null;
  name?: string | null;
};

export type AdapterAuthenticatorPayload = {
  credentialID: string;
  providerAccountId: string;
  userId: string;
  credentialPublicKey: string;
  counter: number;
  credentialDeviceType: string;
  credentialBackedUp: boolean;
  transports?: string | null;
};

export type GoTokensPayload = {
  access_token: string;
  refresh_token: string;
  expires_in?: number;
};

export type GitHubOAuthConfigPayload = {
  enabled: boolean;
  register_allowed?: boolean;
  client_id: string;
  client_secret: string;
};

export type OAuthConfigPayload = GitHubOAuthConfigPayload;

export type LinkOAuthAccountPayload = {
  userId: string;
  provider: string;
  providerAccountId: string;
  type: string;
  access_token?: string;
  refresh_token?: string;
  expires_at?: number;
  token_type?: string;
  scope?: string;
  github_login?: string;
};

export async function adapterGetOAuthConfig(
  provider: "github" | "google" | "facebook" | "apple",
) {
  const result = await fetchUpstream(`internal/auth/oauth/${provider}`, {
    method: "GET",
    headers: adapterHeaders(),
  });
  return unwrap<OAuthConfigPayload>(result);
}

export async function adapterGetGitHubOAuthConfig() {
  return adapterGetOAuthConfig("github");
}

export async function adapterCreateOAuthUser(input: {
  email: string;
  name?: string | null;
  surname?: string | null;
  email_verified?: boolean;
}) {
  const result = await fetchUpstream("internal/auth/users", {
    method: "POST",
    headers: adapterHeaders(),
    body: JSON.stringify({
      email: input.email,
      name: input.name ?? "",
      surname: input.surname ?? "",
      email_verified: Boolean(input.email_verified),
    }),
  });
  return unwrap<AdapterUserPayload>(result);
}

export async function adapterGetOAuthAccountUser(
  provider: string,
  providerAccountId: string,
) {
  const result = await fetchUpstream(
    `internal/auth/accounts?provider=${encodeURIComponent(provider)}&provider_account_id=${encodeURIComponent(providerAccountId)}`,
    { method: "GET", headers: adapterHeaders() },
  );
  return unwrap<AdapterUserPayload>(result);
}

export async function adapterLinkOAuthAccount(input: LinkOAuthAccountPayload) {
  const result = await fetchUpstream("internal/auth/accounts", {
    method: "POST",
    headers: adapterHeaders(),
    body: JSON.stringify({
      userId: input.userId,
      provider: input.provider,
      providerAccountId: input.providerAccountId,
      type: input.type,
      access_token: input.access_token ?? null,
      refresh_token: input.refresh_token ?? null,
      expires_at: input.expires_at ?? null,
      token_type: input.token_type ?? null,
      scope: input.scope ?? null,
      github_login: input.github_login ?? null,
    }),
  });
  await unwrap<{ status: string }>(result);
}

export async function adapterUnlinkOAuthAccount(
  provider: string,
  providerAccountId: string,
) {
  const result = await fetchUpstream(
    `internal/auth/accounts?provider=${encodeURIComponent(provider)}&provider_account_id=${encodeURIComponent(providerAccountId)}`,
    { method: "DELETE", headers: adapterHeaders() },
  );
  await unwrap<{ status: string }>(result);
}

export async function adapterGetUserByEmail(email: string) {
  const result = await fetchUpstream(
    `internal/auth/users/by-email?email=${encodeURIComponent(email)}`,
    { method: "GET", headers: adapterHeaders() },
  );
  return unwrap<AdapterUserPayload>(result);
}

export async function adapterGetUserByUUID(userId: string) {
  const result = await fetchUpstream(`internal/auth/users/${userId}`, {
    method: "GET",
    headers: adapterHeaders(),
  });
  return unwrap<AdapterUserPayload>(result);
}

export async function adapterGetAuthenticator(credentialId: string) {
  const result = await fetchUpstream(
    `internal/auth/authenticators/${encodeURIComponent(credentialId)}`,
    { method: "GET", headers: adapterHeaders() },
  );
  return unwrap<AdapterAuthenticatorPayload>(result);
}

export async function adapterListAuthenticators(userId: string) {
  const result = await fetchUpstream(
    `internal/auth/users/${userId}/authenticators`,
    { method: "GET", headers: adapterHeaders() },
  );
  const data = await unwrap<{ items: AdapterAuthenticatorPayload[] }>(result);
  return data.items ?? [];
}

export async function adapterCreateAuthenticator(
  input: Omit<AdapterAuthenticatorPayload, "userId"> & { userId: string },
) {
  const result = await fetchUpstream("internal/auth/authenticators", {
    method: "POST",
    headers: adapterHeaders(),
    body: JSON.stringify({
      userId: input.userId,
      credentialID: input.credentialID,
      providerAccountId: input.providerAccountId,
      credentialPublicKey: input.credentialPublicKey,
      counter: input.counter,
      credentialDeviceType: input.credentialDeviceType,
      credentialBackedUp: input.credentialBackedUp,
      transports: input.transports ?? null,
    }),
  });
  return unwrap<AdapterAuthenticatorPayload>(result);
}

export async function adapterUpdateAuthenticatorCounter(
  credentialId: string,
  counter: number,
) {
  const result = await fetchUpstream(
    `internal/auth/authenticators/${encodeURIComponent(credentialId)}/counter`,
    {
      method: "PATCH",
      headers: adapterHeaders(),
      body: JSON.stringify({ counter }),
    },
  );
  await unwrap<{ status: string }>(result);
}

export async function adapterDeleteAuthenticator(credentialId: string) {
  const result = await fetchUpstream(
    `internal/auth/authenticators/${encodeURIComponent(credentialId)}`,
    { method: "DELETE", headers: adapterHeaders() },
  );
  await unwrap<{ status: string }>(result);
}

export async function adapterIssueSession(
  userUuid: string,
  authMethod: "passkey" | "oauth" = "oauth",
) {
  const result = await fetchUpstream("internal/auth/session", {
    method: "POST",
    headers: adapterHeaders(),
    body: JSON.stringify({ user_uuid: userUuid, auth_method: authMethod }),
  });
  return unwrap<GoTokensPayload>(result);
}

export async function loginWithPassword(
  email: string,
  password: string,
  totpCode?: string,
  organizationSlug?: string,
  clientIp?: string | null,
  forwardedHost?: string | null,
) {
  const result = await fetchUpstream("auth/login", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...(clientIp ? { "X-Forwarded-For": clientIp } : {}),
      ...(forwardedHost ? { "X-Forwarded-Host": forwardedHost } : {}),
    },
    body: JSON.stringify({
      email,
      password,
      ...(totpCode ? { totp_code: totpCode } : {}),
      ...(organizationSlug ? { organization_slug: organizationSlug } : {}),
    }),
  });
  return unwrap<GoTokensPayload>(result);
}
