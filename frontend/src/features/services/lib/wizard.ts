/**
 * Service wizard steps (TEC-97, F1-05). TEC-181 built steps 1 and 3,
 * TEC-182 steps 2 (parts) and 4 (stock, completion). TEC-291 puts the
 * intake contract right before the completing stock step; it is hidden
 * while the organization's intake_contracts module is off.
 */
export const WIZARD_STEPS = [
  "customer_vehicle",
  "parts",
  "measurement",
  "contract",
  "stock",
] as const;

export type WizardStep = (typeof WIZARD_STEPS)[number];

/** The visible steps: `contract` only with the intake_contracts module. */
export function wizardSteps(contractsEnabled: boolean): readonly WizardStep[] {
  return contractsEnabled
    ? WIZARD_STEPS
    : WIZARD_STEPS.filter((step) => step !== "contract");
}

/**
 * Whether the contract still blocks the steps after it: the service
 * requires an intake contract (contracts.intake_required with the module
 * on) and the linked contract is not executed yet.
 */
export function contractBlocksNext(service: {
  contract_required: boolean;
  contract?: { status: string } | null;
}): boolean {
  return service.contract_required && service.contract?.status !== "executed";
}

/**
 * Whether a step may be opened: step 1 always; every later step needs the
 * draft service, which step 1 creates. With `contractLocked` the steps
 * after the contract stay closed.
 */
export function canOpenStep(
  step: WizardStep,
  hasService: boolean,
  contractLocked = false,
  steps: readonly WizardStep[] = WIZARD_STEPS,
): boolean {
  if (step === "customer_vehicle") return true;
  if (!hasService) return false;
  const contractAt = steps.indexOf("contract");
  return !(
    contractLocked &&
    contractAt >= 0 &&
    steps.indexOf(step) > contractAt
  );
}

export function nextStep(
  step: WizardStep,
  steps: readonly WizardStep[] = WIZARD_STEPS,
): WizardStep | null {
  const i = steps.indexOf(step);
  return i < 0 ? null : (steps[i + 1] ?? null);
}

export function previousStep(
  step: WizardStep,
  steps: readonly WizardStep[] = WIZARD_STEPS,
): WizardStep | null {
  const i = steps.indexOf(step);
  return i > 0 ? steps[i - 1] : null;
}

/** Parses a km input: "" = none, otherwise an integer in 0..5,000,000. */
export function parseKm(raw: string): number | null | "invalid" {
  const v = raw.trim();
  if (v === "") return null;
  if (!/^\d+$/.test(v)) return "invalid";
  const n = Number(v);
  return n <= 5_000_000 ? n : "invalid";
}
