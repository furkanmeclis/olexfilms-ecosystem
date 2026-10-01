import type { Breadcrumb, ErrorEvent } from "@sentry/nextjs";

/**
 * PII scrubbing for error events (the technowide receiver does no masking).
 * Mirrors backend internal/errtrack/scrub.go: e-mails and phone numbers are
 * masked in free text, sensitive keys are filtered, the user keeps only its
 * id and the request keeps only method + URL without query.
 */

const MASK_EMAIL = "[email]";
const MASK_PHONE = "[phone]";
export const MASK_FILTERED = "[Filtered]";

const EMAIL_RE = /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g;
const PHONE_CANDIDATE_RE = /\+?\(?\d[\d\s().-]{6,}\d/g;

const SENSITIVE_KEYS = new Set([
  "email",
  "e_mail",
  "phone",
  "phone_number",
  "phone_e164",
  "phone_national",
  "mobile",
  "gsm",
  "name",
  "full_name",
  "first_name",
  "last_name",
  "surname",
  "username",
  "address",
  "password",
  "token",
  "access_token",
  "refresh_token",
  "authorization",
  "cookie",
  "otp",
  "code",
  "tax_number",
  "national_id",
  "tckn",
  "ip",
  "ip_address",
]);

/** SDK-filled contexts carry no personal data; key filtering would mangle them. */
const SDK_CONTEXTS = new Set([
  "os",
  "runtime",
  "device",
  "trace",
  "browser",
  "app",
  "culture",
  "cloud_resource",
  "nextjs",
]);

function isSensitiveKey(key: string): boolean {
  return SENSITIVE_KEYS.has(key.trim().toLowerCase());
}

const IDENT_CHAR = /[A-Za-z0-9_-]/;

function scrubPhones(input: string): string {
  return input.replace(
    PHONE_CANDIDATE_RE,
    (match: string, offset: number, whole: string) => {
      const before = offset > 0 ? whole[offset - 1] : "";
      const after = whole[offset + match.length] ?? "";
      // Part of an identifier (uuid, hex id) or a clock time → keep.
      if (
        (before && IDENT_CHAR.test(before)) ||
        (after && (IDENT_CHAR.test(after) || after === ":"))
      ) {
        return match;
      }
      const digits = match.replace(/\D/g, "").length;
      return digits >= 10 && digits <= 15 ? MASK_PHONE : match;
    },
  );
}

/** Masks e-mail addresses and phone numbers in a string. */
export function scrubString(value: string): string {
  if (!value) return value;
  return scrubPhones(value.replace(EMAIL_RE, MASK_EMAIL));
}

export function scrubValue(value: unknown, depth = 0): unknown {
  if (depth > 8) return value;
  if (typeof value === "string") return scrubString(value);
  if (Array.isArray(value)) return value.map((v) => scrubValue(v, depth + 1));
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      out[k] = isSensitiveKey(k) ? MASK_FILTERED : scrubValue(v, depth + 1);
    }
    return out;
  }
  return value;
}

function stripQuery(url: string): string {
  const i = url.search(/[?#]/);
  return i >= 0 ? url.slice(0, i) : url;
}

export function scrubBreadcrumb(bc: Breadcrumb): Breadcrumb {
  if (bc.message) bc.message = scrubString(bc.message);
  if (bc.data) {
    const data = scrubValue(bc.data) as Record<string, unknown>;
    // fetch / navigation breadcrumbs: drop query strings.
    for (const key of ["url", "from", "to"]) {
      if (typeof data[key] === "string") {
        data[key] = stripQuery(data[key] as string);
      }
    }
    bc.data = data;
  }
  return bc;
}

/** beforeSend: removes personal data from an event before it is sent. */
export function scrubEvent(event: ErrorEvent): ErrorEvent {
  if (event.message) event.message = scrubString(event.message);
  delete event.server_name;
  event.user = event.user?.id !== undefined ? { id: event.user.id } : undefined;
  for (const ex of event.exception?.values ?? []) {
    if (ex.value) ex.value = scrubString(ex.value);
  }
  if (event.breadcrumbs) {
    event.breadcrumbs = event.breadcrumbs.map(scrubBreadcrumb);
  }
  if (event.tags) {
    for (const [k, v] of Object.entries(event.tags)) {
      event.tags[k] = isSensitiveKey(k)
        ? MASK_FILTERED
        : typeof v === "string"
          ? scrubString(v)
          : v;
    }
  }
  if (event.extra) {
    event.extra = scrubValue(event.extra) as Record<string, unknown>;
  }
  if (event.contexts) {
    for (const [name, ctx] of Object.entries(event.contexts)) {
      if (SDK_CONTEXTS.has(name) || !ctx) continue;
      event.contexts[name] = scrubValue(ctx) as Record<string, unknown>;
    }
  }
  if (event.request) {
    event.request = {
      method: event.request.method,
      url: event.request.url
        ? scrubString(stripQuery(event.request.url))
        : undefined,
    };
  }
  return event;
}
