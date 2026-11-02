/**
 * Shake detection over accelerometer samples (in g, gravity included). Pure:
 * no React Native import, so it runs under `node --test`.
 *
 * A shake is several sharp peaks in a short window. One peak is a rising edge
 * of the magnitude across `threshold` (the magnitude must fall back below
 * `threshold - hysteresis` before the next one counts). Walking stays well
 * under the threshold (about 1.2-1.6 g), and putting the phone on a table
 * gives one or two peaks (impact and bounce), never `minPeaks`.
 */
export type ShakeOptions = {
  /** Magnitude (g) a peak must reach. At rest the magnitude is about 1 g. */
  threshold: number;
  /** The magnitude must drop this far below `threshold` before the next peak counts. */
  hysteresis: number;
  /** Peaks needed inside `windowMs`. */
  minPeaks: number;
  windowMs: number;
  /** Two peaks closer than this are one peak (sensor jitter). */
  minPeakGapMs: number;
  /** No new shake for this long after one fired. */
  cooldownMs: number;
};

export const DEFAULT_SHAKE_OPTIONS: ShakeOptions = {
  threshold: 2.3,
  hysteresis: 0.6,
  minPeaks: 4,
  windowMs: 1200,
  minPeakGapMs: 90,
  cooldownMs: 3000,
};

/** Accelerometer update interval the detector is tuned for (20 Hz). */
export const SHAKE_SAMPLE_INTERVAL_MS = 50;

export type Sample = { x: number; y: number; z: number; t: number };

export function magnitude(s: { x: number; y: number; z: number }) {
  return Math.sqrt(s.x * s.x + s.y * s.y + s.z * s.z);
}

export type ShakeDetector = {
  /** Feeds one sample; true when this sample completes a shake. */
  push: (s: Sample) => boolean;
  reset: () => void;
};

export function createShakeDetector(options: Partial<ShakeOptions> = {}): ShakeDetector {
  const o = { ...DEFAULT_SHAKE_OPTIONS, ...options };
  let peaks: number[] = [];
  let armed = true;
  let lastFired = -Infinity;

  return {
    push(s) {
      if (!Number.isFinite(s.x) || !Number.isFinite(s.y) || !Number.isFinite(s.z)) return false;
      const m = magnitude(s);
      if (m < o.threshold - o.hysteresis) {
        armed = true;
        return false;
      }
      if (m < o.threshold || !armed) return false;
      armed = false;
      if (s.t - lastFired < o.cooldownMs) return false;
      const last = peaks[peaks.length - 1];
      if (last !== undefined && s.t - last < o.minPeakGapMs) return false;
      peaks = peaks.filter((p) => s.t - p <= o.windowMs);
      peaks.push(s.t);
      if (peaks.length < o.minPeaks) return false;
      peaks = [];
      lastFired = s.t;
      return true;
    },
    reset() {
      peaks = [];
      armed = true;
      lastFired = -Infinity;
    },
  };
}
