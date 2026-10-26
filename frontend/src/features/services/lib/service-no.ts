/**
 * Hand-entered service numbers (TEC-1004). Printed warranty cards carry a
 * QR code `https://olexfilms.app/warranty/XXXX`; scanning, pasting the URL
 * or typing the bare number all end in `XXXX`. The backend rule
 * (`NormalizeServiceNo`): 4..32 letters, digits, `-` or `_`, kept as
 * printed (case-sensitive).
 */

export const SERVICE_NO_RE = /^[A-Za-z0-9_-]{4,32}$/;

/**
 * The service number in a scanned or pasted text: the path segment after
 * `/warranty/` of a URL (with or without scheme, any host, `/pdf` or a
 * query after it ignored), else the trimmed text itself.
 */
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
