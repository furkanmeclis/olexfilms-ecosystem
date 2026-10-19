/**
 * Gauge readings (pure, alias-free): the "probe down → steady
 * → commit → probe up" sampler, and unit conversion for the form fields.
 */
import type { GaugeReading } from './curve.ts';

export type { GaugeReading };

export type ThicknessUnit = 'um' | 'mil';

export const UM_PER_MIL = 25.4;

/** Microns → the user's unit: integer µm, mils with one decimal. */
export function fromMicrons(um: number, unit: ThicknessUnit): number {
  return unit === 'mil' ? Math.round((um / UM_PER_MIL) * 10) / 10 : Math.round(um);
}

/** Value as typed into a part field (the form parser accepts `.` and `,`). */
export function formatInputValue(um: number, unit: ThicknessUnit, decimalSeparator = '.'): string {
  const v = fromMicrons(um, unit);
  return unit === 'mil' ? v.toFixed(1).replace('.', decimalSeparator) : String(v);
}

/** Appends a reading to a part's space separated value list. */
export function appendValue(text: string, value: string): string {
  const t = text.trim();
  return t ? `${t} ${value}` : value;
}

export const SAMPLER = {
  /** Valid samples kept in the window. */
  window: 10,
  /** Samples needed before a commit. */
  minSamples: 8,
  /** Max standard deviation (µm) of a steady reading. */
  maxStdUm: 25,
} as const;

export type SamplerState = {
  phase: 'idle' | 'sampling' | 'committed';
  samples: number[];
  material: GaugeReading['material'] | null;
};

export const initialSampler: SamplerState = { phase: 'idle', samples: [], material: null };

export type Commit = { um: number; material: GaugeReading['material']; count: number };

function mean(v: number[]) {
  return v.reduce((s, x) => s + x, 0) / v.length;
}

function std(v: number[]) {
  if (v.length < 2) return 0;
  const m = mean(v);
  return Math.sqrt(v.reduce((s, x) => s + (x - m) ** 2, 0) / (v.length - 1));
}

/**
 * One step of the sampler. A commit happens once per contact: after it the
 * sampler waits for the probe to lift (a reading without microns) before it
 * collects again, so holding the probe still never fills a part twice.
 */
export function stepSampler(
  state: SamplerState,
  reading: Pick<GaugeReading, 'um' | 'material'>,
  opts: { window: number; minSamples: number; maxStdUm: number } = SAMPLER,
): { state: SamplerState; commit: Commit | null } {
  if (reading.um == null || reading.um <= 0) {
    return { state: initialSampler, commit: null };
  }
  if (state.phase === 'committed') return { state, commit: null };

  const sameMaterial = state.material === null || state.material === reading.material;
  const samples = [...(sameMaterial ? state.samples : []), reading.um].slice(-opts.window);
  if (samples.length >= opts.minSamples && std(samples) <= opts.maxStdUm) {
    return {
      state: { phase: 'committed', samples, material: reading.material },
      commit: { um: Math.round(mean(samples)), material: reading.material, count: samples.length },
    };
  }
  return { state: { phase: 'sampling', samples, material: reading.material }, commit: null };
}

/** 0…1 progress of the current contact, for the live indicator. */
export function samplerProgress(state: SamplerState, minSamples: number = SAMPLER.minSamples): number {
  if (state.phase === 'committed') return 1;
  return Math.min(1, state.samples.length / minSamples);
}

/** BLE material → backend substrate_type of a flat reading. */
export function substrateOf(material: GaugeReading['material'] | null | undefined): 'Fe' | 'Al' | undefined {
  return material === 'fe' ? 'Fe' : material === 'al' ? 'Al' : undefined;
}
