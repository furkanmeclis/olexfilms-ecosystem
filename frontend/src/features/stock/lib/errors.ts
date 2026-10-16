import { isApiError } from "@/lib/api";

type T = (key: string, params?: Record<string, string | number>) => string;

/**
 * Message for a failed stock operation: the error code's text
 * (errors.codes.STOCK_RECLASSIFICATION_*, STOCK_SPLIT_*, ...) when the
 * catalog has one, otherwise the API message, or the fallback.
 */
export function stockErrorMessage(
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
