/**
 * VIN rules of the backend (customers.NormalizeVIN, also used by
 * PATCH /v1/services/{uuid} `vin`): spaces, tabs and dashes are dropped, the
 * value is upper-cased, then it must be exactly 17 letters or digits without
 * I, O and Q. No check-digit test (the backend has none either).
 */
export const VIN_LENGTH = 17;

const VIN_RE = /^[A-HJ-NPR-Z0-9]{17}$/;
const ALNUM_RE = /^[A-Z0-9]*$/;
const FORBIDDEN_RE = /[IOQ]/;

export type VinError = "required" | "length" | "forbidden_letters" | "chars";

/** Cleans a VIN like the backend: drops spaces/tabs/dashes, upper-cases. */
export function normalizeVin(raw: string): string {
  return raw.toUpperCase().replace(/[\s-]/g, "");
}

/**
 * Validates a raw VIN. Returns null when valid (or empty and optional),
 * otherwise the first failing rule.
 */
export function validateVin(
  raw: string,
  options?: { required?: boolean },
): VinError | null {
  const vin = normalizeVin(raw);
  if (vin === "") return options?.required ? "required" : null;
  if (!ALNUM_RE.test(vin)) return "chars";
  if (FORBIDDEN_RE.test(vin)) return "forbidden_letters";
  if (vin.length !== VIN_LENGTH) return "length";
  return VIN_RE.test(vin) ? null : "chars";
}

/** i18n key of a VIN validation error. */
export function vinErrorKey(error: VinError): string {
  return `services.vin.errors.${error}`;
}
