import { isApiError } from "@/lib/api";

type T = (key: string, params?: Record<string, string | number>) => string;

/**
 * Message for a failed order call. A validation answer carrying a detail
 * code (PRICE_NOT_FOUND on a line) shows that code's text; otherwise the
 * error message, which parseApiError already localized by error code
 * (errors.codes.ORDER_*), or the generic fallback.
 */
export function orderErrorMessage(
  err: unknown,
  t: T,
  fallback: string,
): string {
  if (!isApiError(err)) return fallback;
  for (const d of err.details ?? []) {
    if (!d.code) continue;
    const key = `errors.codes.${d.code}`;
    const text = t(key);
    if (text !== key) return text;
  }
  const key = `errors.codes.${err.code}`;
  const byCode = t(key);
  if (byCode !== key) return byCode;
  return err.message || fallback;
}
