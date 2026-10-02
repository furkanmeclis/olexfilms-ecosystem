/**
 * Service wizard steps (TEC-97, F1-05). TEC-181 builds steps 1 and 3;
 * steps 2 (parts) and 4 (stock) are placeholders filled by TEC-182.
 */
export const WIZARD_STEPS = [
  "customer_vehicle",
  "parts",
  "measurement",
  "stock",
] as const;

export type WizardStep = (typeof WIZARD_STEPS)[number];

/** Steps whose content is not built yet (TEC-182). */
export const PLACEHOLDER_STEPS: ReadonlySet<WizardStep> = new Set([
  "parts",
  "stock",
]);

/**
 * Whether a step may be opened: step 1 always; every later step needs the
 * draft service, which step 1 creates.
 */
export function canOpenStep(step: WizardStep, hasService: boolean): boolean {
  return step === "customer_vehicle" || hasService;
}

export function nextStep(step: WizardStep): WizardStep | null {
  const i = WIZARD_STEPS.indexOf(step);
  return WIZARD_STEPS[i + 1] ?? null;
}

export function previousStep(step: WizardStep): WizardStep | null {
  const i = WIZARD_STEPS.indexOf(step);
  return i > 0 ? WIZARD_STEPS[i - 1] : null;
}

/** Parses a km input: "" = none, otherwise an integer in 0..5,000,000. */
export function parseKm(raw: string): number | null | "invalid" {
  const v = raw.trim();
  if (v === "") return null;
  if (!/^\d+$/.test(v)) return "invalid";
  const n = Number(v);
  return n <= 5_000_000 ? n : "invalid";
}
