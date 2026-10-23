import { fetch as expoFetch } from 'expo/fetch';
import createClient from 'openapi-fetch';
import { Platform } from 'react-native';

import { useAppStatus } from '@/core/app-status';
import { fromApi, getTokens, setTokens } from '@/core/auth/tokens';
import { API_URL, APP_VERSION, MOBILE_API_VERSION } from '@/core/config';
import { i18n } from '@/core/i18n';

import { ApiError, toApiError } from './errors';
import type { paths } from './schema';

const AUTH_PATHS = ['/v1/mobile/auth/login', '/v1/mobile/auth/refresh'];
/** A 401 here means a wrong password or code, not an expired token: never refresh and resend it. */
const NO_RETRY_PATHS = ['/auth/step-up/'];
/** Refresh this long before the access token expires, so requests never race it. */
const REFRESH_MARGIN_MS = 30_000;

let onSessionExpired: () => void = () => {};
/** The session store registers how to sign out when the refresh chain is gone. */
export function setSessionExpiredHandler(fn: () => void) {
  onSessionExpired = fn;
}

function contractHeaders(): Record<string, string> {
  return {
    Accept: 'application/json',
    'X-Mobile-Api-Version': MOBILE_API_VERSION,
    'X-App-Version': APP_VERSION,
    'Accept-Language': i18n.language,
    'User-Agent': `OlexFilms/${APP_VERSION} (${Platform.OS})`,
  };
}

let refreshing: Promise<boolean> | null = null;

/** Rotates the refresh token once, however many requests ask at the same time. */
export function refreshTokens(): Promise<boolean> {
  refreshing ??= (async () => {
    const t = getTokens();
    if (!t) return false;
    try {
      const res = await fetch(`${API_URL}/v1/mobile/auth/refresh`, {
        method: 'POST',
        headers: { ...contractHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: t.refresh, app_version: APP_VERSION }),
      });
      if (!res.ok) {
        // 401 (expired or REFRESH_TOKEN_REUSED): the device chain is gone.
        if (res.status === 401) onSessionExpired();
        return false;
      }
      const body = await res.json();
      await setTokens(fromApi(body.data));
      return true;
    } catch {
      return false;
    }
  })().finally(() => {
    refreshing = null;
  });
  return refreshing;
}

function withAuth(request: Request) {
  const t = getTokens();
  if (t && !AUTH_PATHS.some((p) => request.url.includes(p))) {
    request.headers.set('Authorization', `Bearer ${t.access}`);
  }
  return request;
}

async function handle426(res: Response) {
  if (res.status !== 426) return;
  const body = await res.clone().json().catch(() => undefined);
  const code = body?.error?.code === 'UPDATE_REQUIRED' ? 'UPDATE_REQUIRED' : 'MOBILE_API_VERSION_UNSUPPORTED';
  // 426 UPDATE_REQUIRED carries { min_version, current_version, store_urls: { ios, android } } in data.
  const urls = body?.data?.store_urls;
  useAppStatus.getState().setUpdateRequired({ code, storeUrl: urls?.[Platform.OS] ?? null });
}

async function authFetch(request: Request): Promise<Response> {
  const retry = request.clone();
  const t = getTokens();
  if (t && t.accessExpiresAt - Date.now() < REFRESH_MARGIN_MS) await refreshTokens();

  let res = await fetch(withAuth(request));
  const noRetry = [...AUTH_PATHS, ...NO_RETRY_PATHS].some((p) => request.url.includes(p));
  if (res.status === 401 && getTokens() && !noRetry) {
    if (await refreshTokens()) res = await fetch(withAuth(retry));
  }
  await handle426(res);
  return res;
}

export const api = createClient<paths>({ baseUrl: API_URL, fetch: authFetch });

/**
 * Panel routes (`/v1/services`, `/v1/customers`, ...) are reached through the
 * mobile mirror `/v1/mobile/panel/*`: same handlers and permissions, mobile
 * token only. Screens call the typed panel path; this rewrites it.
 */
export const PANEL_MIRROR_PREFIX = '/v1/mobile/panel/';
/** Customer sessions reach the portal routes (`/v1/portal/*`) through `/v1/mobile/portal/*`. */
export const PORTAL_MIRROR_PREFIX = '/v1/mobile/portal/';

async function toMirror(request: Request): Promise<Request> {
  const url = new URL(request.url);
  const base = new URL(API_URL);
  const basePath = base.pathname.replace(/\/$/, '');
  const rel = url.pathname.slice(basePath.length);
  if (!rel.startsWith('/v1/') || rel.startsWith('/v1/mobile/')) return request;
  url.pathname = rel.startsWith('/v1/portal/')
    ? basePath + PORTAL_MIRROR_PREFIX + rel.slice('/v1/portal/'.length)
    : basePath + PANEL_MIRROR_PREFIX + rel.slice('/v1/'.length);
  // React Native's Request (whatwg-fetch) has no `body` property, so
  // `new Request(url, request)` would drop it: copy the body explicitly.
  // whatwg-fetch keeps the original body in `_bodyInit` (FormData stays intact).
  let body: BodyInit | undefined;
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    const raw = (request as unknown as { _bodyInit?: BodyInit | null })._bodyInit;
    body = (raw ?? (await request.text())) || undefined;
  }
  return new Request(url.toString(), { method: request.method, headers: request.headers, body, signal: request.signal });
}

api.use({
  async onRequest({ request }) {
    const req = await toMirror(request);
    for (const [k, v] of Object.entries(contractHeaders())) req.headers.set(k, v);
    return req;
  },
});

type Result<T> = { data?: { data: T }; error?: unknown; response: Response };

/**
 * Turns an openapi-fetch call into the envelope's `data`, or throws ApiError
 * with the backend error code. Network failures become NETWORK_ERROR.
 */
export async function unwrap<T>(call: Promise<Result<T>>): Promise<T> {
  let r: Result<T>;
  try {
    r = await call;
  } catch (e) {
    throw new ApiError(0, { code: 'NETWORK_ERROR', message: e instanceof Error ? e.message : String(e) });
  }
  if (r.error !== undefined || !r.response.ok || !r.data) throw toApiError(r.response.status, r.error);
  return r.data.data;
}

/**
 * Multipart POST with React Native FormData file parts (`{ uri, name, type }`)
 * to a panel path (`/v1/...`, sent through the mobile mirror). The global
 * fetch is expo/fetch, which cannot send `uri` parts, so this uses
 * XMLHttpRequest (React Native networking reads the file natively).
 */
export async function uploadForm<T>(path: string, form: FormData): Promise<T> {
  const t0 = getTokens();
  if (t0 && t0.accessExpiresAt - Date.now() < REFRESH_MARGIN_MS) await refreshTokens();
  const url = `${API_URL}${PANEL_MIRROR_PREFIX}${path.replace(/^\/v1\//, '')}`;
  const send = () =>
    new Promise<{ status: number; body: unknown }>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', url);
      for (const [k, v] of Object.entries(contractHeaders())) xhr.setRequestHeader(k, v);
      const t = getTokens();
      if (t) xhr.setRequestHeader('Authorization', `Bearer ${t.access}`);
      xhr.onload = () => {
        let body: unknown;
        try {
          body = JSON.parse(xhr.responseText);
        } catch {
          body = undefined;
        }
        resolve({ status: xhr.status, body });
      };
      xhr.onerror = () => reject(new ApiError(0, { code: 'NETWORK_ERROR', message: 'Network request failed' }));
      xhr.send(form);
    });
  let r = await send();
  if (r.status === 401 && getTokens() && (await refreshTokens())) r = await send();
  if (r.status === 426) {
    const body = r.body as { error?: { code?: string }; data?: { store_urls?: Record<string, string> } } | undefined;
    useAppStatus.getState().setUpdateRequired({
      code: body?.error?.code === 'UPDATE_REQUIRED' ? 'UPDATE_REQUIRED' : 'MOBILE_API_VERSION_UNSUPPORTED',
      storeUrl: body?.data?.store_urls?.[Platform.OS] ?? null,
    });
  }
  if (r.status < 200 || r.status >= 300) throw toApiError(r.status, r.body);
  return (r.body as { data: T }).data;
}

/**
 * POST to a panel path (`/v1/...`, through the mobile mirror) that answers
 * with `text/event-stream` (the AI assistant, TEC-960). Uses `expo/fetch`
 * explicitly: its `Response.body` is a ReadableStream that delivers chunks as
 * they arrive on Hermes / the New Architecture (React Native's XHR-based
 * fetch buffers the whole body). Same auth, refresh and 426 handling as
 * `api`; the caller reads `body` (or `text()` when there is no stream).
 */
export async function panelStream(path: string, body: unknown, signal: AbortSignal) {
  const t0 = getTokens();
  if (t0 && t0.accessExpiresAt - Date.now() < REFRESH_MARGIN_MS) await refreshTokens();
  const url = `${API_URL}${PANEL_MIRROR_PREFIX}${path.replace(/^\/v1\//, '')}`;
  const send = () => {
    const headers: Record<string, string> = {
      ...contractHeaders(),
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    };
    const t = getTokens();
    if (t) headers.Authorization = `Bearer ${t.access}`;
    return expoFetch(url, { method: 'POST', headers, body: JSON.stringify(body ?? {}), signal });
  };
  let res = await send();
  if (res.status === 401 && getTokens() && (await refreshTokens())) res = await send();
  if (res.status === 426) {
    const payload = await res.json().catch(() => undefined);
    useAppStatus.getState().setUpdateRequired({
      code: payload?.error?.code === 'UPDATE_REQUIRED' ? 'UPDATE_REQUIRED' : 'MOBILE_API_VERSION_UNSUPPORTED',
      storeUrl: payload?.data?.store_urls?.[Platform.OS] ?? null,
    });
  }
  return res;
}
