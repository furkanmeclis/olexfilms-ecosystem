/**
 * Zero (foil) calibration of a SmartPTG / NexPTG gauge (pure, alias-free).
 *
 * The official app (docs/smartptg/REVERSE_ENGINEERING.md §6.1) sends no
 * command to calibrate: with a reference foil on the steel (Fe) and then the
 * aluminium (Al) plate it waits for 10 steady frames of that channel and
 * stores, per serial,
 *
 *   offset = curve frequency of the foil thickness − measured frequency
 *
 * which every later frequency of the channel gets added (curve.ts
 * frequencyToMicrons). The foil is 90 µm when the curve file mentions
 * cal1 / cal2 / cal3, else 150 µm.
 */
import { type CurveOffsets, deviceRules, frequencyToMicrons, type GaugeCurve, type Material } from './curve.ts';
import type { GaugeFrame } from './protocol.ts';

export const CALIBRATION = {
  /** Steady frames needed (NexPTGService counter B). */
  stableSamples: 10,
  /** Two consecutive frames are steady when they differ by less than this (Hz). */
  maxStepHz: 10,
  /** Unsteady or off-curve frames before the step fails (counter z). */
  maxErrors: 10,
  /** A step fails when it does not finish in time (q7.d case 2). */
  timeoutMs: 5_000,
  /** The foil point is the curve point within this many µm of the target. */
  toleranceUm: 10,
  /** Without such a point the app uses the 6th point. */
  fallbackIndex: 5,
} as const;

/** Foil thickness of a curve: 90 µm for cal1 / cal2 / cal3 curves, else 150 µm. */
export function calibrationTargetUm(curve: Pick<GaugeCurve, 'calVersion'>): 90 | 150 {
  return curve.calVersion ? 90 : 150;
}

/** NexPTGService.e(material): the curve frequency of the foil point. */
export function calibrationTargetFrequency(curve: GaugeCurve, material: Material): number {
  const target = calibrationTargetUm(curve);
  const points = material === 'fe' ? curve.fe : curve.al;
  let best: { d: number; f: number } | null = null;
  for (const p of points) {
    const d = Math.abs(p.thickness - target);
    if (d <= CALIBRATION.toleranceUm && (!best || d < best.d)) best = { d, f: p.frequency };
  }
  return best?.f ?? (points[CALIBRATION.fallbackIndex] ?? points[points.length - 1]!).frequency;
}

/** The channels a gauge calibrates: Economic (3, 4) has no Al channel. */
export function calibrationSteps(deviceType: number): Material[] {
  return deviceRules(deviceType).measuresAl ? ['fe', 'al'] : ['fe'];
}

export type CalibrationStep = {
  material: Material;
  targetFrequency: number;
  last: number | null;
  /** Frequencies of the steady frames. */
  stable: number[];
  errors: number;
  startedAt: number;
};

export function startCalibrationStep(curve: GaugeCurve, material: Material, now: number): CalibrationStep {
  return { material, targetFrequency: calibrationTargetFrequency(curve, material), last: null, stable: [], errors: 0, startedAt: now };
}

export type CalibrationOutcome =
  | { kind: 'progress'; step: CalibrationStep }
  | { kind: 'done'; offset: number; measured: number; samples: number[] }
  | { kind: 'error'; reason: 'unstable' | 'timeout' };

export function calibrationTimedOut(step: CalibrationStep, now: number): boolean {
  return now - step.startedAt > CALIBRATION.timeoutMs;
}

/** 0…1 progress of a step. */
export function calibrationProgress(step: CalibrationStep): number {
  return Math.min(1, step.stable.length / CALIBRATION.stableSamples);
}

/**
 * One frame of a step. Frames of the other channel or another serial are
 * ignored. The curve is read without an offset: a frame counts only while
 * the probe is on metal (a value), and steady when it is within maxStepHz of
 * the previous frame.
 */
export function stepCalibration(step: CalibrationStep, frame: GaugeFrame, curve: GaugeCurve, now: number): CalibrationOutcome {
  if (calibrationTimedOut(step, now)) return { kind: 'error', reason: 'timeout' };
  if (frame.channel !== step.material || frame.serial !== curve.serial) return { kind: 'progress', step };

  const valid = frequencyToMicrons(step.material, frame.frequency, curve, null) != null;
  const steady = valid && step.last != null && Math.abs(frame.frequency - step.last) < CALIBRATION.maxStepHz;
  const first = valid && step.last == null;
  const next: CalibrationStep = {
    ...step,
    last: frame.frequency,
    stable: steady ? [...step.stable, frame.frequency] : step.stable,
    errors: steady || first ? step.errors : step.errors + 1,
  };
  if (next.stable.length >= CALIBRATION.stableSamples) {
    return { kind: 'done', offset: next.targetFrequency - frame.frequency, measured: frame.frequency, samples: next.stable };
  }
  if (next.errors >= CALIBRATION.maxErrors) return { kind: 'error', reason: 'unstable' };
  return { kind: 'progress', step: next };
}

/* ---------------------------------------------------------------- storage */

/** A calibration as kept on the phone (one per serial) and sent to the backend. */
export type StoredCalibration = {
  serial: number;
  /** Hz offsets; null = the channel was not calibrated. */
  fe: number | null;
  al: number | null;
  targetUm: 90 | 150;
  /** ISO time the calibration finished. */
  calibratedAt: string;
  samples: { fe: number[]; al: number[] };
  /** Idempotency key of the upload (the backend uuid for one read from the backend). */
  clientId: string;
  /** False until the backend stored it. */
  synced: boolean;
  /** Who calibrated (from the backend), null when unknown. */
  by: string | null;
};

/** Offsets to apply; 0 for a channel never calibrated. */
export function offsetsOf(c: StoredCalibration | null | undefined): CurveOffsets | null {
  return c ? { fe: c.fe ?? 0, al: c.al ?? 0 } : null;
}

/** The calibration to use: the newer one; on a tie the backend's (it is shared). */
export function newerCalibration(
  local: StoredCalibration | null,
  remote: StoredCalibration | null,
): StoredCalibration | null {
  if (!local) return remote;
  if (!remote) return local;
  return Date.parse(local.calibratedAt) > Date.parse(remote.calibratedAt) ? local : remote;
}

/** Backend GaugeCalibration → stored form. */
export function fromBackendCalibration(c: {
  uuid: string;
  serial: number;
  fe_offset_hz: number | null;
  al_offset_hz: number | null;
  target_um: number;
  samples: { fe: number[]; al: number[] };
  calibrated_at: string;
  created_by: { name: string };
}): StoredCalibration {
  return {
    serial: c.serial,
    fe: c.fe_offset_hz,
    al: c.al_offset_hz,
    targetUm: c.target_um === 90 ? 90 : 150,
    calibratedAt: c.calibrated_at,
    samples: { fe: c.samples.fe ?? [], al: c.samples.al ?? [] },
    clientId: c.uuid,
    synced: true,
    by: c.created_by.name || null,
  };
}

/** Stored form → backend POST body. */
export function toBackendCalibration(c: StoredCalibration) {
  return {
    serial: c.serial,
    fe_offset_hz: c.fe,
    al_offset_hz: c.al,
    target_um: c.targetUm,
    samples: c.samples,
    client_id: c.clientId,
    calibrated_at: c.calibratedAt,
  };
}

/** RFC 4122 v4 UUID for the upload's idempotency key (not a secret). */
export function uuidV4(random: () => number = Math.random): string {
  const b = Array.from({ length: 16 }, () => Math.floor(random() * 256) & 0xff);
  b[6] = (b[6]! & 0x0f) | 0x40;
  b[8] = (b[8]! & 0x3f) | 0x80;
  const h = b.map((x) => x.toString(16).padStart(2, '0')).join('');
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}
