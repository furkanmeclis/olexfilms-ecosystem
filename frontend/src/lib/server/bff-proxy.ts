import {
  clearAuthSessionCookie,
  getApiTokens,
  persistApiTokens,
  type AuthRealm,
} from "@/lib/server/auth-tokens";
import {
  sharedRefresh,
  type RefreshResult,
} from "@/lib/server/refresh-single-flight";
import {
  clientIpFromHeaders,
  fetchUpstream,
  forwardedHostFromHeaders,
  fetchUpstreamStream,
  type UpstreamResult,
} from "@/lib/server/upstream";
import { acceptLanguageFor } from "@/lib/server/accept-language";

type TokensData = {
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
  access_expires_at?: string;
  refresh_expires_at?: string;
};

type Envelope = {
  success?: boolean;
  data?: TokensData & Record<string, unknown>;
  meta?: unknown;
  error?: unknown;
};

type SessionSwitchData = {
  user_uuid?: string;
  email?: string;
  impersonator_uuid?: string | null;
};

/** Paths whose successful response updates the NextAuth JWT token pair. */
function isAuthTokenPath(realm: AuthRealm, path: string) {
  if (path === "auth/refresh") return true;
  // Organization context and impersonation are panel only (TEC-90).
  if (realm === "portal") return false;
  if (path === "auth/organization-context") return true;
  if (path === "auth/impersonation/stop") return true;
  return path.startsWith("platform/users/") && path.endsWith("/impersonate");
}

/** Public auth endpoints that must not send Bearer. */
const AUTH_PUBLIC_TOKEN_PATHS = new Set(["auth/refresh", "auth/register"]);

function isAuthPublicTokenPath(path: string) {
  return AUTH_PUBLIC_TOKEN_PATHS.has(path);
}

function isLogoutPath(path: string) {
  return path === "auth/logout";
}

function isSessionInvalidatePath(path: string) {
  return path === "auth/password/change" || path === "auth/password/reset";
}

function isRefreshPath(path: string) {
  return path === "auth/refresh";
}

function wantsBinaryStream(path: string) {
  return (
    path.includes("platform/storage/objects/download") ||
    path.includes("platform/storage/objects/preview") ||
    path.startsWith("public/storage/") ||
    // Quote PDFs (tenant download / public share link).
    /^tenant\/quotes\/[^/]+\/pdf$/.test(path) ||
    /^public\/quotes\/[^/]+\/pdf$/.test(path) ||
    // Gotenberg documents (TEC-88): editor preview and tenant downloads.
    path === "platform/document-templates/preview" ||
    /^tenant\/documents\/[^/]+\/download$/.test(path) ||
    // AI read-aloud: stream MP3 as Speaches produces it.
    path === "tenant/ai/voice/speech"
  );
}

function wantsEventStream(request: Request) {
  const accept = request.headers.get("accept") ?? "";
  return accept.includes("text/event-stream");
}

function encodeJson(value: unknown): string {
  return JSON.stringify(value);
}

function parseJson(buffer: ArrayBuffer): Envelope | null {
  try {
    const text = new TextDecoder().decode(buffer);
    if (!text) return null;
    return JSON.parse(text) as Envelope;
  } catch {
    return null;
  }
}

function stripTokens(envelope: Envelope): Envelope {
  if (!envelope.data || typeof envelope.data !== "object") return envelope;
  const data = { ...envelope.data };
  delete data.access_token;
  delete data.refresh_token;
  return {
    ...envelope,
    data: {
      ...data,
      authenticated: true,
    },
  };
}

function resolveAccessMaxAge(data: TokensData | undefined): number | undefined {
  if (typeof data?.expires_in === "number" && data.expires_in > 0) {
    return data.expires_in;
  }
  return undefined;
}

async function persistTokensFromEnvelope(
  realm: AuthRealm,
  envelope: Envelope | null,
) {
  const access = envelope?.data?.access_token;
  const refresh = envelope?.data?.refresh_token;
  if (!access || !refresh) return false;

  const sessionPayload = envelope?.data?.session as
    SessionSwitchData | undefined;
  const existing = await getApiTokens(realm);
  const userId = sessionPayload?.user_uuid ?? existing.userId;
  if (!userId) return false;

  const impersonatorUuid =
    sessionPayload !== undefined
      ? (sessionPayload.impersonator_uuid ?? null)
      : existing.impersonatorUuid;

  await persistApiTokens(realm, {
    accessToken: access,
    refreshToken: refresh,
    userId,
    email: sessionPayload?.email ?? existing.email,
    impersonatorUuid,
    expiresIn: resolveAccessMaxAge(envelope?.data),
    refreshExpiresAt:
      typeof envelope?.data?.refresh_expires_at === "string"
        ? envelope.data.refresh_expires_at
        : null,
  });
  return true;
}

/** One upstream rotation per refresh token, shared by concurrent requests. */
function rotateRefreshToken(
  refreshToken: string,
  forwardedFor: string | null,
  forwardedHost: string | null = null,
): Promise<RefreshResult> {
  return sharedRefresh(refreshToken, async (token) => {
    const headers = new Headers({
      Accept: "application/json",
      "Content-Type": "application/json",
    });
    // Rate limits and session rows key on the client IP (first caller's).
    if (forwardedFor) headers.set("X-Forwarded-For", forwardedFor);
    // Go re-validates the org scope against the domain's brand on refresh.
    if (forwardedHost) headers.set("X-Forwarded-Host", forwardedHost);
    const result = await fetchUpstream("auth/refresh", {
      method: "POST",
      headers,
      body: encodeJson({ refresh_token: token }),
    });
    return { status: result.status, body: result.body };
  });
}

/**
 * `"rejected"`: the API refused the refresh token (the session is over).
 * `"unavailable"`: the API could not be reached, failed (5xx) or rate-limited
 * the refresh (429). The session cookie is kept so a short outage or a
 * restart does not sign everyone out; the caller answers 503 instead of 401.
 */
export type RefreshOutcome = "ok" | "rejected" | "unavailable";

/** Whether an upstream refresh status means "try again later", not "signed out". */
export function isRefreshUnavailableStatus(status: number): boolean {
  return status >= 500 || status === 429;
}

async function refreshViaUpstream(
  realm: AuthRealm,
  forwardedFor: string | null,
  forwardedHost: string | null = null,
): Promise<RefreshOutcome> {
  const { refreshToken, userId } = await getApiTokens(realm);
  if (!refreshToken || !userId) return "rejected";

  // Do not clear the session cookie on failure: a concurrent response may
  // already have written a valid rotated pair, and the browser's own refresh
  // flow signs out when the session is really gone.
  let result: RefreshResult;
  try {
    result = await rotateRefreshToken(
      refreshToken,
      forwardedFor,
      forwardedHost,
    );
  } catch {
    return "unavailable";
  }
  if (isRefreshUnavailableStatus(result.status)) return "unavailable";
  if (result.status >= 400) return "rejected";
  return (await persistTokensFromEnvelope(realm, parseJson(result.body)))
    ? "ok"
    : "rejected";
}

/** JSON error envelope for a backend that is down or restarting. */
export function unavailableResponse(): Response {
  return new Response(
    JSON.stringify({
      success: false,
      error: {
        code: "SERVICE_UNAVAILABLE",
        message: "The service is temporarily unavailable. Try again shortly.",
      },
    }),
    {
      status: 503,
      headers: {
        "Content-Type": "application/json",
        "Retry-After": "5",
        "Cache-Control": "no-store",
      },
    },
  );
}

function buildUpstreamBody(
  path: string,
  method: string,
  incoming: ArrayBuffer,
  refreshToken: string | null,
): BodyInit | null {
  if (method === "GET" || method === "HEAD") return null;

  if (isRefreshPath(path)) {
    return encodeJson({ refresh_token: refreshToken ?? "" });
  }

  if (isLogoutPath(path)) {
    let clientBody: Record<string, unknown> = {};
    try {
      const text = new TextDecoder().decode(incoming);
      if (text) clientBody = JSON.parse(text) as Record<string, unknown>;
    } catch {
      clientBody = {};
    }
    return encodeJson({
      ...clientBody,
      refresh_token: refreshToken ?? clientBody.refresh_token ?? "",
    });
  }

  return incoming.byteLength > 0 ? incoming : null;
}

/**
 * Response headers forwarded to the browser. Content-Length is only kept for
 * untouched binary passthrough bodies: buffered bodies may be rewritten (auth
 * token paths strip tokens, so the JSON gets shorter) and fetch() transparently
 * decompresses encoded bodies, so forwarding the upstream length there makes
 * browsers fail with ERR_CONTENT_LENGTH_MISMATCH. Without it the runtime sets
 * the correct length (or chunked encoding) itself.
 */
function passthroughHeaders(
  upstream: Headers,
  { keepLength = false }: { keepLength?: boolean } = {},
): Headers {
  const out = new Headers();
  const allow = [
    "content-type",
    "content-disposition",
    "cache-control",
    "location",
    "x-request-id",
    "x-content-type-options",
    "content-security-policy",
  ] as const;
  for (const key of allow) {
    const value = upstream.get(key);
    if (value) out.set(key, value);
  }
  const length = upstream.get("content-length");
  if (keepLength && length && !upstream.get("content-encoding")) {
    out.set("content-length", length);
  }
  return out;
}

function streamPassthroughHeaders(upstream: Headers): Headers {
  const out = passthroughHeaders(upstream);
  if (!out.has("content-type")) {
    out.set("Content-Type", "text/event-stream");
  }
  out.set("Cache-Control", "no-cache, no-transform");
  out.set("Connection", "keep-alive");
  out.set("X-Accel-Buffering", "no");
  return out;
}

async function proxyBuffered(
  realm: AuthRealm,
  path: string,
  pathWithQuery: string,
  request: Request,
  headers: Headers,
  body: BodyInit | null,
  refreshToken: string | null,
): Promise<Response> {
  let result: UpstreamResult;
  if (isRefreshPath(path) && refreshToken) {
    let rotated: RefreshResult;
    try {
      rotated = await rotateRefreshToken(
        refreshToken,
        headers.get("X-Forwarded-For"),
        headers.get("X-Forwarded-Host"),
      );
    } catch {
      return unavailableResponse();
    }
    // API down or restarting: keep the session, let the browser retry.
    if (isRefreshUnavailableStatus(rotated.status)) {
      return unavailableResponse();
    }
    result = {
      ...rotated,
      headers: new Headers({ "Content-Type": "application/json" }),
    };
  } else {
    result = await fetchUpstream(pathWithQuery, {
      method: request.method,
      headers,
      body,
    });
  }

  if (
    result.status === 401 &&
    Boolean(refreshToken) &&
    !isAuthPublicTokenPath(path) &&
    !isLogoutPath(path)
  ) {
    const refreshed = await refreshViaUpstream(
      realm,
      headers.get("X-Forwarded-For"),
      headers.get("X-Forwarded-Host"),
    );
    if (refreshed === "unavailable") return unavailableResponse();
    if (refreshed === "ok") {
      const { accessToken: nextAccess } = await getApiTokens(realm);
      if (nextAccess) headers.set("Authorization", `Bearer ${nextAccess}`);
      result = await fetchUpstream(pathWithQuery, {
        method: request.method,
        headers,
        body,
      });
    }
  }

  if (
    isLogoutPath(path) ||
    (result.status < 400 && isSessionInvalidatePath(path))
  ) {
    await clearAuthSessionCookie(realm);
  }

  let responseBody: ArrayBuffer | string = result.body;
  const envelope = parseJson(result.body);

  if (result.status < 400 && isAuthTokenPath(realm, path)) {
    await persistTokensFromEnvelope(realm, envelope);
    if (envelope) {
      responseBody = JSON.stringify(stripTokens(envelope));
    }
  }

  return new Response(responseBody, {
    status: result.status,
    headers: passthroughHeaders(result.headers),
  });
}

async function proxyStream(
  realm: AuthRealm,
  path: string,
  pathWithQuery: string,
  request: Request,
  headers: Headers,
  body: BodyInit | null,
  refreshToken: string | null,
): Promise<Response> {
  const binary = wantsBinaryStream(path);
  if (!binary && !headers.get("Accept")?.includes("text/event-stream")) {
    headers.set("Accept", "text/event-stream");
  }

  let result = await fetchUpstreamStream(pathWithQuery, {
    method: request.method,
    headers,
    body,
    signal: request.signal,
  });

  if (
    result.status === 401 &&
    Boolean(refreshToken) &&
    !isAuthPublicTokenPath(path)
  ) {
    await result.body?.cancel().catch(() => undefined);
    const refreshed = await refreshViaUpstream(
      realm,
      headers.get("X-Forwarded-For"),
      headers.get("X-Forwarded-Host"),
    );
    if (refreshed === "unavailable") return unavailableResponse();
    if (refreshed === "ok") {
      const { accessToken: nextAccess } = await getApiTokens(realm);
      if (nextAccess) headers.set("Authorization", `Bearer ${nextAccess}`);
      result = await fetchUpstreamStream(pathWithQuery, {
        method: request.method,
        headers,
        body,
        signal: request.signal,
      });
    }
  }

  const contentType = result.headers.get("content-type") ?? "";
  if (binary || !contentType.includes("text/event-stream")) {
    if (binary) {
      return new Response(result.body, {
        status: result.status,
        headers: passthroughHeaders(result.headers, { keepLength: true }),
      });
    }
    const chunks: Uint8Array[] = [];
    if (result.body) {
      const reader = result.body.getReader();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        if (value) chunks.push(value);
      }
    }
    const total = chunks.reduce((n, c) => n + c.byteLength, 0);
    const merged = new Uint8Array(total);
    let offset = 0;
    for (const chunk of chunks) {
      merged.set(chunk, offset);
      offset += chunk.byteLength;
    }
    return new Response(merged, {
      status: result.status,
      headers: passthroughHeaders(result.headers),
    });
  }

  return new Response(result.body, {
    status: result.status,
    headers: streamPassthroughHeaders(result.headers),
  });
}

function jsonError(status: number, error: string): Response {
  return new Response(JSON.stringify({ success: false, error }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * Reject path segments that could escape `/v1/*` once the upstream URL is
 * normalised (`..`, encoded slashes) and server-to-server adapter routes that
 * must never be reachable from the browser.
 */
export function isForbiddenProxyPath(segments: string[]): boolean {
  for (const seg of segments) {
    if (seg === "" || seg === "." || seg === "..") return true;
    if (/[/\\%?#]/.test(seg)) return true;
  }
  return segments[0] === "internal";
}

/**
 * Portal BFF allowlist (TEC-90): the customer / fleet portal reaches only
 * these upstream paths; everything else answers 404. OTP verify and login
 * are not here: Auth.js calls them server side, so tokens never reach the
 * browser.
 */
const PORTAL_PATHS: readonly RegExp[] = [
  /^auth\/(refresh|logout|me|profile)$/,
  /^auth\/password\/(forgot|reset|change)$/,
  /^auth\/otp\/request$/,
  /^auth\/sessions(\/[^/]+)?$/,
  /^portal\/.+$/,
  /^public\/.+$/,
];

/** Whether a realm's BFF may forward path (segments already validated). */
export function isRealmPathAllowed(realm: AuthRealm, path: string): boolean {
  if (realm === "portal") return PORTAL_PATHS.some((re) => re.test(path));
  // The panel BFF never forwards portal routes, the phone OTP flow, the
  // mobile API (Bearer passthrough `/api/v1/mobile/*`, TEC-91) or the QR
  // sign-in exchange (Auth.js calls it server side, so the panel token pair
  // never reaches the browser).
  return !(
    path === "portal" ||
    path.startsWith("portal/") ||
    path.startsWith("auth/otp/") ||
    path === "mobile" ||
    path.startsWith("mobile/") ||
    path === "auth/qr/complete"
  );
}

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

/**
 * CSRF defence for cookie-authenticated BFF mutations: browsers send
 * Sec-Fetch-Site / Origin on cross-site requests; only same-origin callers
 * may mutate. Requests without either header (non-browser clients) pass.
 */
export function isCrossSiteMutation(request: Request): boolean {
  if (SAFE_METHODS.has(request.method.toUpperCase())) return false;
  const site = request.headers.get("sec-fetch-site");
  if (site) return site !== "same-origin" && site !== "none";
  const origin = request.headers.get("origin");
  if (!origin || origin === "null") return origin === "null";
  const host =
    request.headers.get("x-forwarded-host") ?? request.headers.get("host");
  try {
    return !host || new URL(origin).host !== host.split(",")[0]!.trim();
  } catch {
    return true;
  }
}

export async function proxyToUpstream(
  realm: AuthRealm,
  pathSegments: string[],
  request: Request,
): Promise<Response> {
  if (isForbiddenProxyPath(pathSegments)) {
    return jsonError(404, "not_found");
  }
  const path = pathSegments.join("/");
  if (!isRealmPathAllowed(realm, path)) {
    return jsonError(404, "not_found");
  }
  if (isCrossSiteMutation(request)) {
    return jsonError(403, "cross_site_request_blocked");
  }
  const url = new URL(request.url);
  const pathWithQuery = `${path}${url.search}`;

  const incomingBody =
    request.method === "GET" || request.method === "HEAD"
      ? new ArrayBuffer(0)
      : await request.arrayBuffer();

  const { accessToken, refreshToken } = await getApiTokens(realm);

  const headers = new Headers();
  const accept = request.headers.get("accept");
  const contentType = request.headers.get("content-type");
  if (accept) headers.set("Accept", accept);
  else headers.set("Accept", "application/json");
  if (contentType) headers.set("Content-Type", contentType);
  // Go rate limits and audit rows key on the client IP; without this every
  // request would share the BFF's address (one global bucket).
  const clientIp = clientIpFromHeaders(request.headers);
  if (clientIp) headers.set("X-Forwarded-For", clientIp);
  // Go resolves the brand from the browser-facing host (K3).
  const forwardedHost = forwardedHostFromHeaders(request.headers);
  if (forwardedHost) headers.set("X-Forwarded-Host", forwardedHost);
  headers.set("Accept-Language", acceptLanguageFor(request)); // TEC-137

  if (accessToken && !isAuthPublicTokenPath(path)) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  const body = buildUpstreamBody(
    path,
    request.method,
    incomingBody,
    refreshToken,
  );

  if (wantsEventStream(request) || wantsBinaryStream(path)) {
    return proxyStream(
      realm,
      path,
      pathWithQuery,
      request,
      headers,
      body,
      refreshToken,
    );
  }

  return proxyBuffered(
    realm,
    path,
    pathWithQuery,
    request,
    headers,
    body,
    refreshToken,
  );
}
