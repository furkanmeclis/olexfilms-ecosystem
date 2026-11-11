/**
 * Phone book of the caller ID (TEC-1208), in the form both native sides
 * read: one `digits<TAB>label` line per number, where the digits are the
 * E.164 number without the plus sign. Lines are unique and ascending by
 * number; iOS rejects a list that is not.
 */
export type CallerEntry = { phone: string; label: string };

/** Digits of an E.164 number (`+905321112233` → `905321112233`), or null when it is not one. */
export function toDigits(phone: string): string | null {
  const digits = phone.trim().replace(/^\+/, '');
  return /^[1-9][0-9]{6,14}$/.test(digits) ? digits : null;
}

/** One line of text: tabs and line breaks would break the file format. */
function cleanLabel(label: string): string {
  return label.replace(/\s+/g, ' ').trim();
}

/** Numeric order without parsing: no leading zeros, so shorter is smaller. */
function byNumber(a: string, b: string): number {
  if (a.length !== b.length) return a.length - b.length;
  return a < b ? -1 : a > b ? 1 : 0;
}

/** Builds the file. Entries without a valid number or label are left out; the first label of a number wins. */
export function buildPhoneBook(entries: readonly CallerEntry[]): { tsv: string; count: number } {
  const labels = new Map<string, string>();
  for (const e of entries) {
    const digits = toDigits(e.phone);
    const label = cleanLabel(e.label);
    if (!digits || !label || labels.has(digits)) continue;
    labels.set(digits, label);
  }
  const numbers = [...labels.keys()].sort(byNumber);
  const tsv = numbers.map((n) => `${n}\t${labels.get(n)}\n`).join('');
  return { tsv, count: numbers.length };
}
