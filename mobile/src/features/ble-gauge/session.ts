/**
 * Connection bookkeeping of the gauge manager (pure, alias-free).
 */

/**
 * Generation counter: every connect attempt takes a ticket; disconnect bumps
 * the counter so a connectToDevice that resolves late is recognised as stale.
 */
export function createGeneration() {
  let current = 0;
  return {
    next: () => ++current,
    bump: () => {
      current += 1;
    },
    isCurrent: (ticket: number) => ticket === current,
  };
}

/**
 * Accepts a serial only after it arrived in `required` consecutive frames, so a
 * single corrupted frame never triggers a curve download for a wrong serial.
 */
export function createSerialConfirm(required = 2) {
  let candidate: number | null = null;
  let count = 0;
  return {
    /** Returns the serial once it is confirmed (on every later frame too), else null. */
    push(serial: number): number | null {
      if (serial === candidate) count += 1;
      else {
        candidate = serial;
        count = 1;
      }
      return count >= required ? serial : null;
    },
    reset() {
      candidate = null;
      count = 0;
    },
  };
}

/**
 * How long to wait for PoweredOn given the adapter state seen so far: while it
 * is Unknown the iOS permission prompt may be on screen, so wait for the user.
 */
export function poweredOnWaitMs(state: string | null): number {
  return state === 'Unknown' || state === 'Resetting' || state === null ? 60_000 : 6_000;
}

/** Error name when PoweredOn never came: a stuck Unknown/Resetting is "not ready", not "off". */
export function poweredOnTimeoutError(state: string | null): 'not_ready' | 'powered_off' {
  return state === 'PoweredOff' ? 'powered_off' : 'not_ready';
}
