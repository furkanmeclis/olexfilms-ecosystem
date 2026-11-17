/**
 * Keyboard-wedge parsing (TEC-1230). A hardware reader types the code and
 * a terminator (Enter, sometimes Tab). Several codes can arrive in one
 * text change when the reader is fast; anything after the last terminator
 * stays in the buffer for the next change.
 */
export function splitWedgeInput(buffer: string): {
  codes: string[];
  rest: string;
} {
  const parts = buffer.split(/[\r\n\t]/);
  const rest = parts.pop() ?? '';
  const codes = parts.map((p) => p.trim()).filter((p) => p.length > 0);
  return { codes, rest };
}

/**
 * Readers configured without a terminator: a buffer that stopped growing
 * for this long is taken as one code. Hardware readers type a whole code
 * within a few milliseconds; nobody types by hand on a hidden input.
 */
export const WEDGE_IDLE_MS = 120;

/** Shortest code taken on the idle flush (stray keys are ignored). */
export const WEDGE_MIN_LENGTH = 3;
