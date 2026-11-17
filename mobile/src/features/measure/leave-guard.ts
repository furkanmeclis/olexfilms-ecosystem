/**
 * Leaving an unsaved measurement (TEC-1222): going back asks "Save as draft?"
 * with Save (writes the phone draft, `drafts.ts`), Delete and Cancel. Pure,
 * no `@/` imports, so node --test runs it.
 */
export type MeasurementInput = {
  plate: string;
  vin: string;
  bodyType: string | null;
  values: Record<string, string>;
  points?: Record<string, unknown>;
};

/** What the user has typed or measured, without blanks, so an untouched field is not a change. */
function fingerprint(m: MeasurementInput | null): string {
  if (!m) return JSON.stringify(['', '', null, {}, []]);
  const values = Object.fromEntries(
    Object.entries(m.values)
      .map(([k, v]) => [k, v.trim()] as const)
      .filter(([, v]) => v !== '')
      .sort(([a], [b]) => a.localeCompare(b)),
  );
  const points = Object.keys(m.points ?? {}).sort();
  return JSON.stringify([m.plate.trim(), m.vin.trim(), m.bodyType ?? null, values, points]);
}

/**
 * True when the screen holds something not on the phone yet: anything typed
 * in a new measurement, or a change to a resumed draft (`initial`).
 */
export function hasUnsavedMeasurement(current: MeasurementInput, initial: MeasurementInput | null): boolean {
  return fingerprint(current) !== fingerprint(initial);
}

/** Buttons of the leave question; Save needs what a draft needs (a plate, a valid VIN or none). */
export function leaveChoices(input: { plateOk: boolean; vinOk: boolean }): ('cancel' | 'save' | 'discard')[] {
  return input.plateOk && input.vinOk ? ['cancel', 'save', 'discard'] : ['cancel', 'discard'];
}
