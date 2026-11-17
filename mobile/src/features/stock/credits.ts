/**
 * TEC-1221 (kredili kod): usage credits of a serial piece unit (barcode).
 * Every selection in a service takes one credit, "ürünü bitirdim" takes the
 * rest; rolls and fixed barcodes carry no credits (null). No `@/` imports:
 * node --test runs this file directly.
 */

/** API bound of a credit amount (product default, plotter line, super admin increase). */
export const MAX_CREDITS = 1000;

export type CreditsSummary = { credits_total: number; credits_remaining: number };

/** "remaining / total" of a unit, or null when the unit carries no credits. */
export function creditsText(c: CreditsSummary | null | undefined): string | null {
  return c ? `${c.credits_remaining} / ${c.credits_total}` : null;
}

/** Whether the unit can still be selected in a service (a unit without credits always can). */
export function hasCreditsLeft(c: CreditsSummary | null | undefined): boolean {
  return !c || c.credits_remaining > 0;
}

/** Only serial piece products give their barcodes credits (not a roll, not a fixed barcode). */
export function productUsesCredits(p: { unit_type: string; uses_fixed_barcode: boolean }): boolean {
  return !p.uses_fixed_barcode && p.unit_type !== 'roll_meter';
}

/** A credit amount typed by hand: a whole number 1..1000, else null. */
export function parseCredits(raw: string): number | null {
  const v = raw.trim();
  if (!/^[0-9]{1,4}$/.test(v)) return null;
  const n = Number(v);
  return n >= 1 && n <= MAX_CREDITS ? n : null;
}
