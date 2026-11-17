/** TEC-1226 pure helpers (node tests import this file directly). */

/** A damage document needs a photo before it is sent for approval. */
export function needsPhoto(a: {
  type: string;
  photos?: readonly unknown[];
}): boolean {
  return a.type === "damage" && (a.photos?.length ?? 0) === 0;
}

/** "2,5" / "2.5" → "2.5"; invalid or not above 0 → undefined. */
export function normalizeMeters(raw: string): string | undefined {
  const v = raw.trim().replace(",", ".");
  if (!v || !/^\d+(\.\d{1,2})?$/.test(v)) return undefined;
  return Number(v) > 0 ? v : undefined;
}
