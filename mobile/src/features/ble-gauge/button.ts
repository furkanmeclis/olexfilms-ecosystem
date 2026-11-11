/**
 * The gauge's button with the probe in the air (user decision 11.11). The
 * gauge marks frames 3 / 4 while the button is down: one marked frame for a
 * short press, then one every ~0.4–0.6 s while held (live log 11.11, SN 32475).
 * On a surface the button keeps its NexPTG meaning (save the reading now,
 * readings.ts). In the air:
 * - short press → `short` (measuring form: next part), sent once the marks stop;
 * - held ≥ BUTTON.longMs → `long` (measuring form: remove the last value), once.
 * Marks right after the probe left a surface are ignored (lifting while pressing).
 */
export type ButtonEvent = 'short' | 'long';

export const BUTTON = {
  /** No marked frame for this long ends a press. */
  endGapMs: 800,
  /** Held this long is a long press. */
  longMs: 1_500,
  /** Air marks this soon after a surface contact belong to that contact. */
  contactGraceMs: 700,
} as const;

type Timer = { set: (fn: () => void, ms: number) => unknown; clear: (handle: unknown) => void };

const realTimer: Timer = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
};

export type ButtonTracker = {
  /** One frame: `button` = type 3 / 4, `air` = no thickness (probe off the surface). */
  feed: (frame: { button: boolean; air: boolean }, at: number) => void;
  reset: () => void;
};

export function createButtonTracker(onEvent: (e: ButtonEvent) => void, timer: Timer = realTimer): ButtonTracker {
  let lastContactAt = -Infinity;
  let press: { start: number; long: boolean } | null = null;
  let endHandle: unknown = null;

  const stopTimer = () => {
    if (endHandle != null) timer.clear(endHandle);
    endHandle = null;
  };

  return {
    feed({ button, air }, at) {
      if (!air) {
        lastContactAt = at;
        if (press) {
          stopTimer();
          press = null;
        }
        return;
      }
      if (!button || at - lastContactAt < BUTTON.contactGraceMs) return;
      if (!press) press = { start: at, long: false };
      const current = press;
      if (!current.long && at - current.start >= BUTTON.longMs) {
        current.long = true;
        onEvent('long');
      }
      stopTimer();
      endHandle = timer.set(() => {
        endHandle = null;
        if (press === current && !current.long) onEvent('short');
        if (press === current) press = null;
      }, BUTTON.endGapMs);
    },
    reset() {
      stopTimer();
      press = null;
      lastContactAt = -Infinity;
    },
  };
}
