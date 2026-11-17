/**
 * Stock entry scan helpers (TEC-1224). Pure, no `@/` imports, so node --test runs it.
 */

/**
 * The barcode of the entry line a scanned code stands for. The backend
 * upper-cases codes, so a scanner that sends lower case still finds its
 * line; the line's own spelling is returned for the API calls.
 */
export function entryLineBarcode(lines: readonly { barcode: string }[], code: string): string | undefined {
  const wanted = code.trim().toUpperCase();
  if (!wanted) return undefined;
  return lines.find((l) => l.barcode.toUpperCase() === wanted)?.barcode;
}
