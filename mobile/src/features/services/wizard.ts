/**
 * Service intake wizard rules, kept free of React so plain node tests run them.
 *
 * Step order follows the legacy mobile app (customer + vehicle, applied parts,
 * products, summary with number / notes / status) on top of the new backend:
 * the draft service is opened when the first step is left (stock units and
 * items need the service), so the later steps work on that draft. With the
 * photo_standard module the intake photos sit right after the first step, as
 * on the web wizard (TEC-500), and a missing required angle keeps the later
 * steps closed.
 */
import { isValidServiceNo } from './service-no.ts';

export const WIZARD_STEPS = ['customer', 'photos', 'parts', 'products', 'summary'] as const;
export type WizardStep = (typeof WIZARD_STEPS)[number];

/** Visible steps: `photos` only while the photo standard applies to the service. */
export function wizardSteps(photos: boolean): readonly WizardStep[] {
  return WIZARD_STEPS.filter((s) => photos || s !== 'photos');
}

export function isWizardStep(v: unknown): v is WizardStep {
  return typeof v === 'string' && (WIZARD_STEPS as readonly string[]).includes(v);
}

/**
 * State the gates read. `partsCount` / `itemCount` left out are not checked
 * (the legacy app required both: at least one body part before products, at
 * least one product before the summary; mobile-only, the backend does not).
 */
export type GateCtx = {
  hasService: boolean;
  photosMissing?: boolean;
  partsCount?: number;
  itemCount?: number;
  steps?: readonly WizardStep[];
};

/** Why a step is closed: no draft yet, missing photos, no parts, no products. */
export type StepBlocker = 'service' | 'photos' | 'parts' | 'items';

/** The step each blocker sends the user back to. */
export const BLOCKER_STEP: Record<StepBlocker, WizardStep> = {
  service: 'customer',
  photos: 'photos',
  parts: 'parts',
  items: 'products',
};

/** First gate before `step` that is not met, in step order (null = open). */
export function stepBlocker(step: WizardStep, ctx: GateCtx): StepBlocker | null {
  if (step === 'customer') return null;
  if (!ctx.hasService) return 'service';
  const steps = ctx.steps ?? WIZARD_STEPS;
  const at = steps.indexOf(step);
  const gates: [WizardStep, StepBlocker, boolean][] = [
    ['photos', 'photos', !!ctx.photosMissing],
    ['parts', 'parts', ctx.partsCount === 0],
    ['products', 'items', ctx.itemCount === 0],
  ];
  for (const [gate, reason, failing] of gates) {
    const gateAt = steps.indexOf(gate);
    if (failing && gateAt >= 0 && at > gateAt) return reason;
  }
  return null;
}

/** Whether a blocker still holds (its message hides once the user fixed it). */
export function isBlocking(reason: StepBlocker, ctx: GateCtx): boolean {
  if (reason === 'service') return !ctx.hasService;
  if (reason === 'photos') return !!ctx.photosMissing;
  if (reason === 'parts') return ctx.partsCount === 0;
  return ctx.itemCount === 0;
}

/** Why the user cannot move on from `step` (null = may continue). */
export function leaveBlocker(step: WizardStep, ctx: GateCtx): StepBlocker | null {
  const next = nextStep(step, ctx.steps ?? WIZARD_STEPS);
  return next ? stepBlocker(next, ctx) : null;
}

/** A step opens when it is visible and no earlier gate blocks it. */
export function canOpenStep(step: WizardStep, ctx: GateCtx): boolean {
  if (step !== 'customer' && !(ctx.steps ?? WIZARD_STEPS).includes(step)) return false;
  return stepBlocker(step, ctx) === null;
}

export function nextStep(step: WizardStep, steps: readonly WizardStep[] = WIZARD_STEPS): WizardStep | null {
  const i = steps.indexOf(step);
  return i < 0 ? null : (steps[i + 1] ?? null);
}

export function previousStep(step: WizardStep, steps: readonly WizardStep[] = WIZARD_STEPS): WizardStep | null {
  const i = steps.indexOf(step);
  return i > 0 ? steps[i - 1] : null;
}

/**
 * The step actually shown: a hidden step falls to the next visible one and a
 * locked step shows the gate that locks it (photos, then parts, then
 * products), e.g. the summary falls back to products when the last product
 * is removed.
 */
export function resolveStep(wanted: WizardStep, ctx: GateCtx & { steps: readonly WizardStep[] }): WizardStep {
  if (!ctx.hasService) return 'customer';
  let step = wanted;
  if (!ctx.steps.includes(step)) step = step === 'photos' ? 'parts' : 'summary';
  const blocker = stepBlocker(step, ctx);
  return blocker ? BLOCKER_STEP[blocker] : step;
}

/** Where a resumed draft opens: the first step after customer / vehicle. */
export function resumeStep(steps: readonly WizardStep[]): WizardStep {
  return nextStep('customer', steps) ?? 'customer';
}

/** Kilometre input: "" = none; digits only, 0..5,000,000 (ServiceCreateInput.km). */
export function parseKm(raw: string): number | null | 'invalid' {
  const v = raw.replace(/[\s.,]/g, '');
  if (v === '') return null;
  if (!/^\d+$/.test(v)) return 'invalid';
  const n = Number(v);
  return n <= 5_000_000 ? n : 'invalid';
}

export type CustomerStepError = 'customer' | 'vehicle' | 'km';

/** What still blocks leaving the first step (null = may continue). */
export function customerStepError(v: {
  customer: unknown;
  vehicle: { car_model?: unknown } | null;
  km: string;
}): CustomerStepError | null {
  if (!v.customer) return 'customer';
  // The backend copies brand / model from the vehicle and refuses one without a model.
  if (!v.vehicle || !v.vehicle.car_model) return 'vehicle';
  if (parseKm(v.km) === 'invalid') return 'km';
  return null;
}

/** Parts chosen on the parts step that a product may carry (its category's available_parts). */
export function partsForProduct(selected: readonly string[], available: readonly string[] | null | undefined): string[] {
  const allowed = new Set(available ?? []);
  return selected.filter((p) => allowed.has(p));
}

/** Union of the parts the items already carry, in first-seen order. */
export function partsFromItems(items: readonly { applied_parts?: readonly string[] | null }[] | null | undefined): string[] {
  const out: string[] = [];
  for (const it of items ?? []) for (const p of it.applied_parts ?? []) if (!out.includes(p)) out.push(p);
  return out;
}

/**
 * Parts the parts step starts with: the selection stored on this device, or
 * when there is none (another device, an empty selection) the parts the
 * draft's items already carry, so a resumed draft with items is not locked.
 */
export function initialParts(
  stored: readonly string[] | null | undefined,
  items: readonly { applied_parts?: readonly string[] | null }[] | null | undefined,
): string[] {
  return stored?.length ? [...stored] : partsFromItems(items);
}

/** Comma separated parts of a route param (`?parts=a,b`). */
export function parseParts(raw: string | string[] | undefined): string[] {
  const s = Array.isArray(raw) ? raw.join(',') : (raw ?? '');
  return s
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean);
}

/**
 * Status the summary step leaves the service in: the current one (keep the
 * draft) or one of the transitions the backend offers the caller. Cancelling
 * stays on the detail screen.
 */
export function finishChoices<S extends string>(current: S, available: readonly S[]): S[] {
  return [current, ...available.filter((s) => s !== current && s !== 'cancelled')];
}

/** Forward order of the service statuses (cancelled is not a step forward). */
const FORWARD = ['draft', 'pending', 'processing', 'ready', 'completed'];

/**
 * Status the summary step preselects: the first forward transition that is
 * neither the draft nor the completion (completing deducts stock, so the user
 * picks it on purpose), else the current status.
 */
export function defaultFinishChoice<S extends string>(current: S, available: readonly S[]): S {
  const at = FORWARD.indexOf(current);
  const next = available
    .filter((s) => s !== current && s !== 'draft' && s !== 'completed' && s !== 'cancelled')
    .filter((s) => FORWARD.indexOf(s) > at)
    .sort((a, b) => FORWARD.indexOf(a) - FORWARD.indexOf(b));
  return next[0] ?? current;
}

/**
 * i18n key (dealer namespace) of the summary button: it names what happens,
 * "Save as draft" when the draft stays, the transition otherwise.
 */
export function finishButtonKey(current: string, choice: string): string {
  if (choice !== current) return `transition.${choice}`;
  return current === 'draft' ? 'wizard.save_draft' : 'wizard.save_keep';
}

/**
 * Whether leaving the wizard would lose something: before the draft exists
 * any pick or kilometre, afterwards a summary field that differs from the
 * service (the same diff the save would send).
 */
export function wizardDirty(v: {
  service: Editable | null;
  form: SummaryForm;
  picked: boolean;
}): boolean {
  if (!v.service) return v.picked || v.form.km.trim() !== '';
  return servicePatch(v.service, v.form) !== null;
}

export type FinishBlocker = 'needs_items';

/** The backend refuses to complete a service without items (400 on `items`). */
export function finishBlocker(choice: string, itemCount: number): FinishBlocker | null {
  return choice === 'completed' && itemCount === 0 ? 'needs_items' : null;
}

export type SummaryForm = {
  km: string;
  notes: string;
  serviceNo: string;
  measure: boolean;
};

export type SummaryError = 'km' | 'service_no';

export function summaryError(f: SummaryForm): SummaryError | null {
  if (parseKm(f.km) === 'invalid') return 'km';
  const no = f.serviceNo.trim();
  if (no && !isValidServiceNo(no)) return 'service_no';
  return null;
}

type Editable = {
  km: number | null;
  notes: string | null;
  service_no: string;
  has_measurement: boolean;
  vin: string | null;
  service_no_editable?: boolean;
};

export type ServicePatch = {
  km?: number | null;
  notes?: string | null;
  service_no?: string;
  has_measurement?: boolean;
};

/**
 * PATCH body with only the changed fields (ServiceUpdateInput); null when
 * nothing changed. An empty number keeps the current one; the measurement
 * needs the VIN.
 */
export function servicePatch(s: Editable, f: SummaryForm): ServicePatch | null {
  const body: ServicePatch = {};
  const km = parseKm(f.km);
  if (km !== 'invalid' && km !== s.km) body.km = km;
  const notes = f.notes.trim() || null;
  if (notes !== (s.notes?.trim() || null)) body.notes = notes;
  const no = f.serviceNo.trim();
  if (no && no !== s.service_no && s.service_no_editable !== false) body.service_no = no;
  const measure = f.measure && !!s.vin;
  if (measure !== s.has_measurement) body.has_measurement = measure;
  return Object.keys(body).length ? body : null;
}

/** Summary form filled from the service (the number field stays empty: keep). */
export function summaryFromService(s: Editable): SummaryForm {
  return {
    km: s.km === null ? '' : String(s.km),
    notes: s.notes ?? '',
    serviceNo: '',
    measure: s.has_measurement,
  };
}
