/**
 * Scan payloads printed on the TEC-202 labels: location QR
 * `OFW:LOC:<full_code>`, unit QR `OFW:UNIT:<barcode>`. Anything else is
 * a bare barcode, location code, SKU or short code the server resolves
 * (TEC-203).
 */
export const LOCATION_QR_PREFIX = "OFW:LOC:";
export const UNIT_QR_PREFIX = "OFW:UNIT:";

export type ScanKind = "location" | "unit" | "other";

/**
 * Normalizes what a hardware scanner or a person typed: trims, drops
 * control characters a scanner may append (tab, CR) and upper-cases a
 * lower-case QR prefix. The payload after the prefix is kept as is.
 */
export function normalizeScan(raw: string): string {
  const code = raw.replace(/[\u0000-\u001f\u007f]/g, "").trim();
  const upper = code.toUpperCase();
  for (const prefix of [LOCATION_QR_PREFIX, UNIT_QR_PREFIX]) {
    if (upper.startsWith(prefix)) return prefix + code.slice(prefix.length);
  }
  return code;
}

/** What the code looks like before the server resolves it. */
export function scanKind(code: string): ScanKind {
  if (code.startsWith(LOCATION_QR_PREFIX)) return "location";
  if (code.startsWith(UNIT_QR_PREFIX)) return "unit";
  return "other";
}

/** The unit barcode inside an `OFW:UNIT:` QR, else the code itself. */
export function unitBarcode(code: string): string {
  return code.startsWith(UNIT_QR_PREFIX)
    ? code.slice(UNIT_QR_PREFIX.length)
    : code;
}
