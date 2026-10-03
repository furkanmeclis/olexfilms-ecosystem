import { isApiError } from "@/lib/api";

type T = (key: string, params?: Record<string, string | number>) => string;

/**
 * The translated message of a warehouse error code (detail codes first,
 * then the top-level code), else `fallback`.
 */
export function warehouseErrorMessage(
  err: unknown,
  t: T,
  fallback: string,
): string {
  if (!isApiError(err)) return fallback;
  for (const d of err.details ?? []) {
    if (!d.code) continue;
    const key = `warehouse.errors.${d.code}`;
    const text = t(key);
    if (text !== key) return text;
  }
  const key = `warehouse.errors.${err.code}`;
  const byCode = t(key);
  if (byCode !== key) return byCode;
  return fallback;
}

/** Number of pages for a total (at least one). */
export function pageCount(total: number, size: number): number {
  return Math.max(1, Math.ceil(total / size));
}
