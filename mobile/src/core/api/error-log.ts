/**
 * In-memory ring buffer of the last failed API calls, read by shake-to-report
 * feedback. Only method, URL path (no query), status and time: never headers,
 * tokens or bodies. Pure: no React Native import.
 */
export type ApiErrorLogEntry = { method: string; path: string; status: number; at: string };

const MAX = 20;
let entries: ApiErrorLogEntry[] = [];

/** Path of a request URL relative to the API base path, without query or fragment. */
export function apiErrorPath(url: string, apiUrl: string) {
  let path: string;
  try {
    path = new URL(url).pathname;
  } catch {
    path = url.split(/[?#]/)[0];
  }
  let base = '';
  try {
    base = new URL(apiUrl).pathname.replace(/\/$/, '');
  } catch {
    // Not a URL: keep the full path.
  }
  return base && path.startsWith(`${base}/`) ? path.slice(base.length) : path;
}

export function logApiError(e: { method: string; path: string; status: number }, now = new Date()) {
  entries = [...entries, { method: e.method.toUpperCase(), path: e.path.split(/[?#]/)[0], status: e.status, at: now.toISOString() }].slice(-MAX);
}

/** Oldest first. */
export function recentApiErrors(): ApiErrorLogEntry[] {
  return [...entries];
}

export function clearApiErrors() {
  entries = [];
}
