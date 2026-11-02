/**
 * Pure logic of the shared picker sheet: search folding, filtering, when to
 * show a search box and how options group into sections. No React Native
 * imports, so it runs under node:test.
 */

/** Lists longer than this get a search box (server lists always do). */
export const SEARCH_THRESHOLD = 7;

/** Data-driven choices up to this many stay inline; longer ones open a sheet. */
export const INLINE_MAX = 5;

export type SearchableOption = {
  key: string;
  title: string;
  subtitle?: string;
  /** Extra text the search matches but the row does not show (codes, city…). */
  keywords?: string;
  /** Section label; consecutive options with the same label share a header. */
  section?: string;
};

/**
 * Lower case without marks, so "İstanbul", "istanbul" and "ISTANBUL" match,
 * as do "Şişli" / "sisli" and Arabic text with or without harakat.
 */
export function foldText(s: string): string {
  return s
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .toLowerCase()
    .replace(/ı/g, 'i')
    .replace(/\s+/g, ' ')
    .trim();
}

/** Every word of the query appears somewhere in the fields. */
export function matchesQuery(fields: readonly (string | null | undefined)[], query: string): boolean {
  const words = foldText(query).split(' ').filter(Boolean);
  if (!words.length) return true;
  const hay = foldText(fields.filter(Boolean).join(' '));
  return words.every((w) => hay.includes(w));
}

/** Options whose title, subtitle or keywords match the query; order kept. */
export function filterOptions<T extends SearchableOption>(options: readonly T[], query: string): T[] {
  if (!foldText(query)) return [...options];
  return options.filter((o) => matchesQuery([o.title, o.subtitle, o.keywords], query));
}

/** Server-searched lists always show the box; local ones from SEARCH_THRESHOLD + 1 options. */
export function needsSearch(count: number, serverSearch: boolean): boolean {
  return serverSearch || count > SEARCH_THRESHOLD;
}

/** A data-driven choice of up to INLINE_MAX options stays inline. */
export function fitsInline(count: number): boolean {
  return count <= INLINE_MAX;
}

export type PickerRow<T> =
  | { kind: 'header'; key: string; title: string }
  | { kind: 'option'; key: string; option: T };

/** Flat rows for the list: a header before each run of options of one section. */
export function pickerRows<T extends SearchableOption>(options: readonly T[]): PickerRow<T>[] {
  const rows: PickerRow<T>[] = [];
  let current: string | undefined;
  options.forEach((o, i) => {
    if (o.section && o.section !== current) rows.push({ kind: 'header', key: `h:${i}:${o.section}`, title: o.section });
    current = o.section;
    rows.push({ kind: 'option', key: `o:${o.key}`, option: o });
  });
  return rows;
}
