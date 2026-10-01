/**
 * Plate helpers mirroring backend/internal/platform/geo/plate.go: the
 * format regex runs on the compact plate (upper case, separators removed).
 * Backend regexes are RE2; they are a subset of JS regex syntax, so the same
 * expression validates in the browser.
 */
export function normalizePlate(raw: string): string {
  return raw
    .trim()
    .toLocaleUpperCase("en-US")
    .replace(/[\s\-._·]/g, "");
}

export function matchPlate(regex: string, plate: string): boolean {
  const compact = normalizePlate(plate);
  if (!compact) return false;
  try {
    return new RegExp(regex, "u").test(compact);
  } catch {
    return false;
  }
}

/** Groups a compact plate like its input mask ("99 AAA 9999") for display. */
export function formatPlate(plate: string, mask: string): string {
  const compact = normalizePlate(plate);
  if (!mask) return compact;
  const groups = mask
    .split(/[\s-]+/)
    .filter(Boolean)
    .map((g) => g.length);
  const sep = mask.includes("-") ? "-" : " ";
  const out: string[] = [];
  let i = 0;
  for (const len of groups) {
    if (i >= compact.length) break;
    out.push(compact.slice(i, i + len));
    i += len;
  }
  if (i < compact.length) out.push(compact.slice(i));
  return out.join(sep);
}
