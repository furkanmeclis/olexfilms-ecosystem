/**
 * Module / limit events. `parseApiError` emits one whenever the API answers
 * with FEATURE_DISABLED (403: the module is off for the organization, 409: it
 * is off one level up) or LIMIT_REACHED (reserved, no endpoint emits it), so
 * listeners can react without per-mutation handling.
 */
export type LimitEventCode =
  "LIMIT_REACHED" | "FEATURE_DISABLED" | "SUBSCRIPTION_READ_ONLY";

export type LimitEventDetail = {
  code: LimitEventCode;
  feature?: string;
  limit?: number;
  used?: number;
  tolerance?: number;
};

const EVENT_NAME = "olexfilms:limit";

export function isLimitEventCode(code: string): code is LimitEventCode {
  return (
    code === "LIMIT_REACHED" ||
    code === "FEATURE_DISABLED" ||
    code === "SUBSCRIPTION_READ_ONLY"
  );
}

export function emitLimitEvent(detail: LimitEventDetail) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent<LimitEventDetail>(EVENT_NAME, { detail }),
  );
}

export function subscribeLimitEvents(cb: (detail: LimitEventDetail) => void) {
  if (typeof window === "undefined") return () => {};
  const handler = (event: Event) =>
    cb((event as CustomEvent<LimitEventDetail>).detail);
  window.addEventListener(EVENT_NAME, handler);
  return () => window.removeEventListener(EVENT_NAME, handler);
}

function numberDetail(value: string | undefined) {
  if (value === undefined || value === "") return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
}

/** Builds the event payload from an error body's details list. */
export function limitDetailFrom(
  code: LimitEventCode,
  details: Array<{ field?: string; message?: string }>,
): LimitEventDetail {
  const map: Record<string, string> = {};
  for (const d of details) {
    if (d.field && d.message !== undefined) map[d.field] = d.message;
  }
  return {
    code,
    feature: map.feature,
    limit: numberDetail(map.limit),
    used: numberDetail(map.used),
    tolerance: numberDetail(map.tolerance),
  };
}

/** True when a query failed because the module is off for the organization. */
export function isFeatureDisabledError(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    (error as { code?: string }).code === "FEATURE_DISABLED"
  );
}

/** True for any limit or feature-disabled API error. */
export function isLimitError(error: unknown): boolean {
  const code =
    typeof error === "object" && error !== null
      ? (error as { code?: string }).code
      : undefined;
  return typeof code === "string" && isLimitEventCode(code);
}
