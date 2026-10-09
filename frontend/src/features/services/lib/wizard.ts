/**
 * Service wizard steps (TEC-97, F1-05). TEC-181 built steps 1 and 3,
 * TEC-182 steps 2 (parts) and 4 (stock, completion). TEC-291 puts the
 * intake contract right before the completing stock step; it is hidden
 * while the organization's intake_contracts module is off. TEC-500: the
 * intake photos follow the customer / vehicle step while the
 * photo_standard module is on.
 */
export const WIZARD_STEPS = [
  "customer_vehicle",
  "photos",
  "parts",
  "measurement",
  "contract",
  "stock",
] as const;

export type WizardStep = (typeof WIZARD_STEPS)[number];

/**
 * The visible steps: `contract` only with the intake_contracts module,
 * `photos` only with the photo_standard module.
 */
export function wizardSteps(
  contractsEnabled: boolean,
  photosEnabled = false,
): readonly WizardStep[] {
  return WIZARD_STEPS.filter(
    (step) =>
      (contractsEnabled || step !== "contract") &&
      (photosEnabled || step !== "photos"),
  );
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
 * after the contract stay closed, with `photosLocked` (a required intake
 * angle is missing, TEC-500) the steps after the photos.
 */
export function canOpenStep(
  step: WizardStep,
  hasService: boolean,
  contractLocked = false,
  steps: readonly WizardStep[] = WIZARD_STEPS,
  photosLocked = false,
): boolean {
  if (step === "customer_vehicle") return true;
  if (!hasService) return false;
  const at = steps.indexOf(step);
  const after = (gate: WizardStep) => {
    const gateAt = steps.indexOf(gate);
    return gateAt >= 0 && at > gateAt;
  };
  return !(
    (contractLocked && after("contract")) ||
    (photosLocked && after("photos"))
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
