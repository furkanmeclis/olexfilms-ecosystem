/**
 * Draft part selection of a service (TEC-182). The service row has no
 * part column (TEC-179 keeps applied_parts per item), so the step 2 choice
 * is kept in this browser tab until it is applied to the items of step 4.
 * Storage may be missing or blocked: every access is guarded.
 */
const KEY = "service-wizard-parts:";

export function readStoredParts(uuid: string): string[] | null {
  try {
    const raw = window.sessionStorage.getItem(KEY + uuid);
    if (!raw) return null;
    const v: unknown = JSON.parse(raw);
    return Array.isArray(v) && v.every((p) => typeof p === "string") ? v : null;
  } catch {
    return null;
  }
}

/** Stores the selection; null forgets it (e.g. after completion). */
export function storeParts(uuid: string, parts: string[] | null) {
  try {
    if (parts === null) window.sessionStorage.removeItem(KEY + uuid);
    else window.sessionStorage.setItem(KEY + uuid, JSON.stringify(parts));
  } catch {
    // ignore: the selection then lives only in memory
  }
}
