/**
 * Split family helpers (TEC-1079, API TEC-1078): a roll and the parts cut
 * from it (`<roll>-3M`, `<roll>-3M-2`, `<roll>-2.5M`, legacy `-S<n>`). Pure,
 * no `@/` imports, so node --test runs it directly.
 */

export type FamilyStatus = 'on_hand' | 'sold' | 'used' | 'in_transit' | 'other';
type Tone = 'neutral' | 'progress' | 'done' | 'problem';

/**
 * Whether a stock search text can be a unit barcode: one word with a digit,
 * at least 4 characters, at most 64 (the API limit). Only then the family of
 * that code is asked for; a product name search never is.
 */
export function looksLikeUnitCode(q: string): boolean {
  const s = q.trim();
  return s.length >= 4 && s.length <= 64 && !/\s/.test(s) && /\d/.test(s);
}

/** Pill tone of a family member's status, relative to the active organization. */
export function familyStatusTone(s: FamilyStatus): Tone {
  if (s === 'on_hand') return 'done';
  if (s === 'in_transit') return 'progress';
  if (s === 'used') return 'problem';
  return 'neutral';
}

/**
 * Route of another barcode in the same stack as the unit screen at
 * `pathname` (`/depo/stock/unit/A` -> `/depo/stock/unit/B`), so a part opens
 * where the roll was opened.
 */
export function siblingUnitPath(pathname: string, barcode: string): string {
  const base = pathname.replace(/\/unit\/[^/]*\/?$/, '');
  return `${base}/unit/${encodeURIComponent(barcode)}`;
}

type Cut = { uuid: string; new_barcode: string };

/**
 * New parts of an assignment response (`split` of a scan, `splits` of an
 * applied suggestion), newest first, merged in front of the ones already
 * shown; a replayed split is not repeated. Keeps the last `max`.
 */
export function mergeCuts<T extends Cut>(
  shown: T[],
  res: { split?: T | null; splits?: T[] | null },
  max = 10,
): T[] {
  const fresh = [...(res.splits ?? []), ...(res.split ? [res.split] : [])].reverse();
  if (!fresh.length) return shown;
  const seen = new Set<string>();
  return [...fresh, ...shown].filter((c) => !seen.has(c.uuid) && (seen.add(c.uuid), true)).slice(0, max);
}
