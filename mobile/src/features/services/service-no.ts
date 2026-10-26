/**
 * Hand-entered service numbers (TEC-1004): the printed warranty card's QR is
 * https://olexfilms.app/warranty/XXXX; scanning, pasting the URL or typing
 * the number all end in XXXX. Backend rule (NormalizeServiceNo): 4..32
 * letters, digits, '-' or '_', kept as printed.
 */
export const SERVICE_NO_RE = /^[A-Za-z0-9_-]{4,32}$/;

/** The path segment after /warranty/ of a URL, else the trimmed text. */
export function extractServiceNo(text: string): string {
  const raw = text.trim();
  const match = /(?:^|\/)warranty\/([^/?#\s]+)/i.exec(raw);
  if (!match) return raw;
  try {
    return decodeURIComponent(match[1]).trim();
  } catch {
    return match[1].trim();
  }
}

export function isValidServiceNo(no: string): boolean {
  return SERVICE_NO_RE.test(no.trim());
}
