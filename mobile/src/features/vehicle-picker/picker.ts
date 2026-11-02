/**
 * Vehicle picker logic: brand → model → year steps, the "frequently used"
 * section and the year range of a model. Plain data + functions: no React and
 * no `@/` imports (runs directly under node --test).
 */

export type PickerStep = 'brand' | 'model' | 'year';
export const PICKER_STEPS: readonly PickerStep[] = ['brand', 'model', 'year'];

export type BrandChoice = { uuid: string; name: string };
export type ModelChoice = { uuid: string; name: string; yearStart?: number | null; yearStop?: number | null };

/** Picker value: a model implies its brand; the year is optional. */
export type VehicleValue = { brand: BrandChoice | null; model: ModelChoice | null; year: number | null };

export const EMPTY_VEHICLE: VehicleValue = { brand: null, model: null, year: null };

/** Oldest year offered when the model has no production start (legacy `/lookups/years`). */
export const OLDEST_YEAR = 1975;

/** A value with brand and model (what a vehicle record needs). */
export function isComplete(v: VehicleValue | null | undefined): v is VehicleValue & { brand: BrandChoice; model: ModelChoice } {
  return Boolean(v?.brand && v.model);
}

/** "Brand Model" of a value; empty when nothing is picked. */
export function vehicleTitle(v: VehicleValue | null | undefined): string {
  return [v?.brand?.name, v?.model?.name].filter(Boolean).join(' ');
}

/** Step to open the picker on: the first one still missing, the year step for a complete value. */
export function startStep(v: VehicleValue | null | undefined): PickerStep {
  if (!v?.brand) return 'brand';
  if (!v.model) return 'model';
  return 'year';
}

/** Steps the user may jump back to (done ones and the current one). */
export function reachableSteps(v: VehicleValue, current: PickerStep): Set<PickerStep> {
  const out = new Set<PickerStep>(['brand', current]);
  if (v.brand) out.add('model');
  if (v.brand && v.model) out.add('year');
  return out;
}

/** The step before `s` (null on the first). */
export function previousStep(s: PickerStep): PickerStep | null {
  const i = PICKER_STEPS.indexOf(s);
  return i > 0 ? PICKER_STEPS[i - 1] : null;
}

/** Picking a brand keeps model and year only when it is the same brand. */
export function pickBrand(v: VehicleValue, brand: BrandChoice): VehicleValue {
  if (v.brand?.uuid === brand.uuid) return { ...v, brand };
  return { brand, model: null, year: null };
}

/** Picking a model keeps the year when it is inside the model's production years. */
export function pickModel(v: VehicleValue, model: ModelChoice, now = new Date()): VehicleValue {
  const keep = v.year != null && yearOptions(model, now).includes(v.year);
  return { ...v, model, year: keep ? v.year : null };
}

/**
 * Years offered for a model, newest first: its production span clamped to
 * next year at most; OLDEST_YEAR when the start is unknown. A reversed span
 * (bad catalog data) falls back to the full range instead of offering nothing.
 */
export function yearOptions(model: Pick<ModelChoice, 'yearStart' | 'yearStop'> | null | undefined, now = new Date()): number[] {
  const max = now.getFullYear() + 1;
  let hi = Math.min(model?.yearStop ?? max, max);
  let lo = Math.max(model?.yearStart ?? OLDEST_YEAR, 1900);
  if (lo > hi) {
    lo = OLDEST_YEAR;
    hi = max;
  }
  const out: number[] = [];
  for (let y = hi; y >= lo; y--) out.push(y);
  return out;
}

/** "2015–2020", "2015–" or "" for a model's production span. */
export function yearSpan(start?: number | null, stop?: number | null): string {
  if (!start && !stop) return '';
  if (!start) return `–${stop}`;
  return `${start}–${stop ?? ''}`;
}

export type FrequentRef = { uuid: string };

export type PickerRow<T> =
  | { kind: 'header'; key: string; section: 'frequent' | 'all' }
  | { kind: 'item'; key: string; item: T; frequent: boolean };

/**
 * Rows of a brand / model list. Without a search and with frequent rows the
 * list is: "Frequently used" header, the frequent items in their usage order,
 * "All" header, every item. The frequent items come from the catalog page
 * when it has them, else from `frequent` itself (`fromFrequent`), so a
 * frequent row shows even before its catalog page loads. A search (or no
 * frequent data, e.g. an older backend) gives the plain list.
 */
export function buildRows<T extends { uuid: string }, F extends FrequentRef>(
  all: readonly T[],
  frequent: readonly F[] | null | undefined,
  searching: boolean,
  fromFrequent: (f: F) => T,
): PickerRow<T>[] {
  const plain = all.map((item) => ({ kind: 'item' as const, key: `all:${item.uuid}`, item, frequent: false }));
  if (searching || !frequent?.length) return plain;
  const byUuid = new Map(all.map((i) => [i.uuid, i]));
  const seen = new Set<string>();
  const top: PickerRow<T>[] = [];
  for (const f of frequent) {
    if (!f?.uuid || seen.has(f.uuid)) continue;
    seen.add(f.uuid);
    top.push({ kind: 'item', key: `top:${f.uuid}`, item: byUuid.get(f.uuid) ?? fromFrequent(f), frequent: true });
  }
  if (!top.length) return plain;
  return [
    { kind: 'header', key: 'h:frequent', section: 'frequent' },
    ...top,
    { kind: 'header', key: 'h:all', section: 'all' },
    ...plain,
  ];
}

/**
 * Frequent rows from a response that may come from an older backend: a
 * missing or malformed list is no frequent data, never a crash.
 */
export function frequentList<F extends FrequentRef>(raw: unknown): F[] {
  if (!Array.isArray(raw)) return [];
  return raw.filter((r): r is F => Boolean(r) && typeof (r as FrequentRef).uuid === 'string');
}
