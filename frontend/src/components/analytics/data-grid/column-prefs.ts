import type { SortingState } from "@tanstack/react-table";

/** Per-user DataGrid layout (TEC-1211), kept in localStorage. */
export type GridPrefs = {
  /** Column ids in display order. */
  order?: string[];
  /** Hidden column ids. */
  hidden?: string[];
  pinFirst?: boolean;
  sorting?: SortingState;
  groupBy?: string | null;
};

const VERSION = "v1";

export function gridPrefsKey(
  persistKey: string,
  userId: string | null | undefined,
): string {
  return `olex.analytics.grid.${VERSION}:${userId || "anon"}:${persistKey}`;
}

export function readGridPrefs(key: string | null): GridPrefs | null {
  if (!key || typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as unknown;
    return parsed && typeof parsed === "object" ? (parsed as GridPrefs) : null;
  } catch {
    return null;
  }
}

export function writeGridPrefs(key: string | null, prefs: GridPrefs): void {
  if (!key || typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, JSON.stringify(prefs));
  } catch {
    // quota / private mode: preferences are a convenience only
  }
}

export function clearGridPrefs(key: string | null): void {
  if (!key || typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(key);
  } catch {
    // ignore
  }
}

/**
 * Saved order reconciled with the current columns: unknown ids dropped,
 * new columns appended in their declared position order.
 */
export function reconcileOrder(
  saved: readonly string[] | undefined,
  ids: readonly string[],
): string[] {
  if (!saved?.length) return [...ids];
  const known = saved.filter((id) => ids.includes(id));
  const missing = ids.filter((id) => !known.includes(id));
  return [...known, ...missing];
}
