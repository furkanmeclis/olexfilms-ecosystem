/**
 * Stock step rules (TEC-182). The backend checks the same (TEC-179
 * AddItem); the client blocks what it can see so the dealer gets the error
 * before the request.
 */

/** Meters of a cut: up to 8 integer and 2 fractional digits (NUMERIC(10,2)). */
const METERS_RE = /^\d{1,8}([.,]\d{1,2})?$/;

export type MetersError = "required" | "format" | "positive" | "exceeds";
export type QuantityError = "required" | "format" | "exceeds";

/** Parses meters ("12,5" or "12.5") to hundredths; null when malformed. */
function toCents(raw: string): number | null {
  const v = raw.trim();
  if (!METERS_RE.test(v)) return null;
  const [int, frac = ""] = v.replace(",", ".").split(".");
  return Number(int) * 100 + Number(frac.padEnd(2, "0"));
}

/** Normalised meters for the API ("12.50"), or null when invalid. */
export function normalizeMeters(raw: string): string | null {
  const c = toCents(raw);
  if (c === null || c <= 0) return null;
  return `${Math.floor(c / 100)}.${String(c % 100).padStart(2, "0")}`;
}

/** Validates a cut against the meters left on the roll. */
export function validateMeters(
  raw: string,
  remaining: string | null | undefined,
): MetersError | null {
  if (raw.trim() === "") return "required";
  const c = toCents(raw);
  if (c === null) return "format";
  if (c <= 0) return "positive";
  const rest = remaining == null ? null : toCents(remaining);
  if (rest !== null && c > rest) return "exceeds";
  return null;
}

/** Validates pieces of a fixed barcode against the pieces on hand. */
export function validateQuantity(
  raw: string,
  onHand: number,
): QuantityError | null {
  const v = raw.trim();
  if (v === "") return "required";
  if (!/^\d{1,6}$/.test(v) || Number(v) < 1) return "format";
  if (Number(v) > onHand) return "exceeds";
  return null;
}

export type PickerUnit = {
  unit_kind: "serial" | "fixed";
  initial_meters: string | null;
  product: { unit_type: string };
};

export type UnitMode = "roll" | "fixed" | "piece";

/** roll: meters are entered; fixed: pieces; piece: the serial unit itself. */
export function unitMode(u: PickerUnit): UnitMode {
  if (u.unit_kind === "fixed") return "fixed";
  if (u.product.unit_type === "roll_meter" || u.initial_meters !== null) {
    return "roll";
  }
  return "piece";
}

/** API codes with their own message in services.stock.errors. */
export const STOCK_ERROR_CODES = [
  "SERVICE_UNIT_NOT_AVAILABLE",
  "SERVICE_UNIT_IN_USE",
  "SERVICE_NOT_EDITABLE",
  "SERVICE_INVALID_TRANSITION",
  "ORGANIZATION_READ_ONLY",
] as const;

export function stockErrorKey(code: string | undefined): string {
  return (STOCK_ERROR_CODES as readonly string[]).includes(code ?? "")
    ? `services.stock.errors.${code}`
    : "services.stock.errors.generic";
}
