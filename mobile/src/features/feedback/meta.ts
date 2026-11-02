/**
 * Shape and cleaning of the per-record metadata (`meta.json`). Pure: no React
 * Native import, so it runs under `node --test`.
 *
 * Nothing secret may leave the device in a feedback ZIP: no tokens, cookies,
 * passwords, Authorization headers, request/response bodies or query values.
 * The collector never reads those; `sanitizeMeta` is the second line.
 */

export type ApiErrorEntry = {
  method: string;
  /** URL path only, never the query string. */
  path: string;
  /** HTTP status; 0 when the request never got an answer. */
  status: number;
  at: string;
};

export type FeedbackMeta = {
  id: string;
  created_at: string;
  timezone: string | null;
  utc_offset_minutes: number;
  note: string;
  files: { original: string; annotated: string | null };
  location: {
    pathname: string;
    segments: string[];
    /** Dynamic route segments (`[uuid]`) and their values. */
    route_params: Record<string, string>;
    /** Other search params: names only, values are dropped. */
    query_param_keys: string[];
    history: string[];
    space: string | null;
  };
  session: {
    session_kind: string | null;
    user_uuid: string | null;
    organization_uuid: string | null;
    organization_type: string | null;
    membership_role: string | null;
    roles: string[];
    permission_count: number;
  };
  app: {
    version: string;
    build: string | null;
    bundle_id: string | null;
    ota_enabled: boolean;
    ota_channel: string | null;
    ota_runtime_version: string | null;
    ota_update_id: string | null;
    ota_embedded: boolean | null;
    environment: 'development' | 'production';
    api_url: string;
  };
  device: {
    brand: string | null;
    model: string | null;
    os: string | null;
    os_version: string | null;
    screen: { width: number; height: number; scale: number; font_scale: number };
    language: string;
    rtl: boolean;
    theme_preference: string;
    color_scheme: string | null;
    network: { type: string | null; connected: boolean | null; internet: boolean | null } | null;
    /** No battery module ships in the app; kept for the ZIP format. */
    battery: null;
  };
  api_errors: ApiErrorEntry[];
};

/** Keys never written to a ZIP, at any depth (matched case-insensitively, `-`/`_` ignored). */
const FORBIDDEN_KEY =
  /token|cookie|password|passwd|secret|authorization|bearer|credential|api_?key|session_?id|^body$|request_?body|response_?body|^query$|^search$/i;

/** True for a key that must not appear in exported metadata. */
export function isForbiddenKey(key: string) {
  return FORBIDDEN_KEY.test(key.replace(/-/g, '_'));
}

/** Drops the query string and fragment from a path or URL. */
export function stripQuery(path: string) {
  const cut = path.search(/[?#]/);
  return cut === -1 ? path : path.slice(0, cut);
}

/** Removes `Bearer xxx` and JWT-looking strings that slipped into free text (the note). */
function scrubText(s: string) {
  return s
    .replace(/bearer\s+[\w\-.~+/]+=*/gi, 'Bearer [removed]')
    .replace(/\beyJ[\w-]{8,}\.[\w-]{8,}\.[\w-]+/g, '[removed]');
}

/**
 * Deep copy without forbidden keys; strings that look like URLs or paths lose
 * their query string. Applied to every `meta.json` and to `feedback.json`.
 */
export function sanitizeMeta<T>(value: T): T {
  const walk = (v: unknown, key: string | null): unknown => {
    if (typeof v === 'string') {
      if (key === 'note') return scrubText(v);
      return /^(https?:\/\/|\/)/.test(v) ? stripQuery(v) : v;
    }
    if (Array.isArray(v)) return v.map((x) => walk(x, key));
    if (v && typeof v === 'object') {
      const out: Record<string, unknown> = {};
      for (const [k, x] of Object.entries(v)) {
        if (isForbiddenKey(k)) continue;
        out[k] = walk(x, k);
      }
      return out;
    }
    return v;
  };
  return walk(value, null) as T;
}

/**
 * Splits expo-router params into the dynamic segments of the current route
 * (kept with their values: record ids) and everything else (query params:
 * names only). `segments` is `useSegments()`, e.g. `['(app)', 'bayi', 'services', '[uuid]']`.
 */
export function splitRouteParams(
  segments: string[],
  params: Record<string, string | string[] | undefined>,
): { route_params: Record<string, string>; query_param_keys: string[] } {
  const dynamic = new Set(
    segments.map((s) => /^\[(?:\.\.\.)?([^\]]+)\]$/.exec(s)?.[1]).filter((s): s is string => !!s),
  );
  const route_params: Record<string, string> = {};
  const query_param_keys: string[] = [];
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined) continue;
    if (dynamic.has(k) && !isForbiddenKey(k)) route_params[k] = Array.isArray(v) ? v.join('/') : v;
    else query_param_keys.push(k);
  }
  return { route_params, query_param_keys: query_param_keys.sort() };
}

/** Appends a visited path to the history (no repeats in a row, newest last, at most `max`). */
export function pushHistory(history: string[], path: string, max = 10): string[] {
  const clean = stripQuery(path);
  if (history[history.length - 1] === clean) return history;
  return [...history, clean].slice(-max);
}
