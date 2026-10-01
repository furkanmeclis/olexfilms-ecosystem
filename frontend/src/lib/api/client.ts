import createClient, { type Middleware } from "openapi-fetch";

import { apiConfig } from "@/config/api";
import type { paths } from "@/generated/api";

import { ApiError, emitApiError, parseApiError } from "./errors";
import { runStepUpEnsure } from "@/features/step-up-engine/lib/step-up-interceptor";

type Envelope<T> = {
  success?: boolean;
  data?: T;
  meta?: { request_id?: string };
};

/**
 * `unavailable`: offline, API down/restarting (5xx) or rate-limited (429).
 * The session is kept; only `rejected` signs the user out.
 */
type RefreshOutcome = "ok" | "rejected" | "unavailable";

let refreshPromise: Promise<RefreshOutcome> | null = null;
let orgContextPromise: Promise<boolean> | null = null;
let onAuthFailure: (() => void | Promise<void>) | null = null;

/** Tenant slug from `/t/{slug}/...` when the SPA is under a tenant shell. */
function tenantSlugFromLocation(): string | null {
  if (typeof window === "undefined") return null;
  const match = window.location.pathname.match(/^\/t\/([^/]+)/);
  return match?.[1] ? decodeURIComponent(match[1]) : null;
}

export function setAuthFailureHandler(
  handler: (() => void | Promise<void>) | null,
) {
  onAuthFailure = handler;
}

async function refreshSession(): Promise<RefreshOutcome> {
  let response: Response;
  try {
    response = await fetch(`${apiConfig.baseUrl}/v1/auth/refresh`, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      credentials: "include",
      body: JSON.stringify({}),
    });
  } catch {
    // Offline or the server is restarting: keep the session.
    return "unavailable";
  }

  if (response.status >= 500 || response.status === 429) return "unavailable";
  if (!response.ok) return "rejected";

  const json = (await response.json()) as Envelope<{ authenticated?: boolean }>;
  return (json.data?.authenticated ?? json.success) ? "ok" : "rejected";
}

function enqueueRefresh() {
  if (!refreshPromise) {
    refreshPromise = refreshSession().finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise;
}

async function switchOrganizationContext(slug: string): Promise<boolean> {
  const response = await fetch(
    `${apiConfig.baseUrl}/v1/auth/organization-context`,
    {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      credentials: "include",
      body: JSON.stringify({ organization_slug: slug }),
    },
  );
  return response.ok;
}

function enqueueOrgContext(slug: string) {
  if (!orgContextPromise) {
    orgContextPromise = switchOrganizationContext(slug).finally(() => {
      orgContextPromise = null;
    });
  }
  return orgContextPromise;
}

const bffMiddleware: Middleware = {
  async onRequest({ request }) {
    const headers = new Headers(request.headers);
    if (!headers.has("Accept")) {
      headers.set("Accept", "application/json");
    }
    return new Request(request, {
      headers,
      credentials: "include",
    });
  },

  async onResponse({ request, response }) {
    if (response.status === 403) {
      const payload = (await response
        .clone()
        .json()
        .catch(() => null)) as { error?: { code?: string } } | null;
      const code = payload?.error?.code;
      if (code === "STEP_UP_REQUIRED") {
        const ok = await runStepUpEnsure();
        if (ok) {
          return fetch(
            new Request(request, {
              headers: request.headers,
              credentials: "include",
            }),
          );
        }
      }
      if (code === "ORGANIZATION_CONTEXT_REQUIRED") {
        const slug = tenantSlugFromLocation();
        const url = new URL(request.url);
        if (
          slug &&
          !url.pathname.endsWith("/auth/organization-context") &&
          !url.pathname.endsWith("/auth/refresh")
        ) {
          const ok = await enqueueOrgContext(slug);
          if (ok) {
            return fetch(
              new Request(request, {
                headers: request.headers,
                credentials: "include",
              }),
            );
          }
        }
      }
    }

    if (response.status !== 401) {
      return response;
    }

    const url = new URL(request.url);
    if (
      url.pathname.endsWith("/auth/login") ||
      url.pathname.endsWith("/auth/refresh") ||
      url.pathname.endsWith("/auth/logout")
    ) {
      return response;
    }

    const outcome = await enqueueRefresh();
    if (outcome === "unavailable") return response;
    if (outcome === "rejected") {
      await onAuthFailure?.();
      return response;
    }

    return fetch(
      new Request(request, {
        headers: request.headers,
        credentials: "include",
      }),
    );
  },
};

export const apiClient = createClient<paths>({
  baseUrl: apiConfig.baseUrl,
  credentials: "include",
});

apiClient.use(bffMiddleware);

type FetchLikeResult = {
  data?: unknown;
  error?: unknown;
  response: Response;
};

export async function unwrap<T>(
  result: FetchLikeResult,
  options?: { silent?: boolean },
): Promise<T> {
  const { data, error, response } = result;

  if (!response.ok || error) {
    const apiError = parseApiError(response.status, error ?? data, {
      emitLimitEvent: !options?.silent,
    });
    if (!options?.silent && apiError.code !== "STEP_UP_REQUIRED") {
      emitApiError(apiError);
    }
    throw apiError;
  }

  const envelope = data as Envelope<T> | T | undefined;
  if (
    envelope &&
    typeof envelope === "object" &&
    "data" in envelope &&
    (envelope as Envelope<T>).data !== undefined
  ) {
    return (envelope as Envelope<T>).data as T;
  }

  return data as T;
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

export async function fetchSession(): Promise<{
  authenticated: boolean;
  has_access: boolean;
  has_refresh: boolean;
  user_id: string | null;
}> {
  const response = await fetch("/api/auth/session", {
    method: "GET",
    credentials: "include",
    headers: { Accept: "application/json" },
    cache: "no-store",
  });

  if (!response.ok) {
    return {
      authenticated: false,
      has_access: false,
      has_refresh: false,
      user_id: null,
    };
  }

  const session = (await response.json()) as {
    user?: { id?: string };
    error?: string;
  };

  const authenticated = Boolean(session.user && !session.error);

  return {
    authenticated,
    has_access: authenticated,
    has_refresh: authenticated,
    user_id: session.user?.id ?? null,
  };
}
