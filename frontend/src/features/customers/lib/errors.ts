import { isApiError } from "@/lib/api";

type T = (key: string, params?: Record<string, string | number>) => string;

/**
 * Message for a failed customer / vehicle call: the error code's text
 * (errors.codes.CUSTOMER_*, ALREADY_MEMBER, INVALID_PLATE, ...) when the
 * catalog has one, otherwise the (already localized) API message, or the
 * fallback.
 */
export function customerErrorMessage(
  err: unknown,
  t: T,
  fallback: string,
): string {
  if (!isApiError(err)) return fallback;
  const key = `errors.codes.${err.code}`;
  const byCode = t(key);
  if (byCode !== key) return byCode;
  return err.message || fallback;
}
