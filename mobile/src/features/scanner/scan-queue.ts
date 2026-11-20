/**
 * TEC-1255: FIFO queue for scanned codes. A reader or the camera can deliver
 * the next code while the previous one is still being sent; the old lock
 * dropped it silently. Codes now wait in order, one runs at a time, and a
 * stuck request gives up after `timeoutMs` so the queue moves on.
 *
 * Kept free of React / native imports so plain node tests can use it.
 */

/** Same code again within this window is the camera seeing it twice, not a second scan. */
export const REPEAT_MS = 2500;

/** A scan request that has not answered by then is reported as timed out. */
export const SCAN_TIMEOUT_MS = 10_000;

/** More than this many waiting codes means something is wrong (a stuck trigger): refuse further codes. */
export const MAX_PENDING = 50;

export class ScanTimeoutError extends Error {
  constructor() {
    super('scan timed out');
    this.name = 'ScanTimeoutError';
  }
}

export type ScanSettled<R> = { ok: true; value: R } | { ok: false; error: unknown };

export type ScanQueueState = { active: string | null; pending: number };

export type PushResult = 'queued' | 'empty' | 'repeat' | 'full';

export type ScanQueueOptions<R> = {
  /** Handles one code; the signal aborts when the code times out or the queue is cleared. */
  run: (code: string, signal: AbortSignal) => Promise<R>;
  /** Called once per code, in scan order, unless the queue was cleared meanwhile. */
  onSettled: (code: string, result: ScanSettled<R>) => void;
  onStart?: (code: string) => void;
  onChange?: (state: ScanQueueState) => void;
  repeatMs?: number;
  timeoutMs?: number;
  maxPending?: number;
  now?: () => number;
};

export type ScanQueue = {
  /** `repeat: false` (reader trigger, typed code) skips the same-code window. */
  push: (raw: string, opts?: { repeat?: boolean }) => PushResult;
  /** Drops waiting codes and ignores the running one (scanner closed). */
  clear: () => void;
  /** Forgets the last code so the same code counts again (manual entry, card closed). */
  resetRepeat: () => void;
  /**
   * The running code now waits for the user (a confirmation over the feed),
   * not for the network: its time limit stops.
   */
  holdTimeout: () => void;
  state: () => ScanQueueState;
};

export function createScanQueue<R>(o: ScanQueueOptions<R>): ScanQueue {
  const repeatMs = o.repeatMs ?? REPEAT_MS;
  const timeoutMs = o.timeoutMs ?? SCAN_TIMEOUT_MS;
  const maxPending = o.maxPending ?? MAX_PENDING;
  const now = o.now ?? (() => Date.now());

  const waiting: string[] = [];
  let active: string | null = null;
  let last = { code: '', at: 0 };
  // Bumped by clear(): a result from before it is dropped.
  let generation = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let ctrl: AbortController | undefined;

  const state = (): ScanQueueState => ({ active, pending: waiting.length });
  const emit = () => o.onChange?.(state());

  const stopTimer = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
  };

  const next = () => {
    if (active !== null) return;
    const code = waiting.shift();
    if (code === undefined) {
      emit();
      return;
    }
    const gen = generation;
    const abort = new AbortController();
    active = code;
    ctrl = abort;
    let settled = false;
    const finish = (result: ScanSettled<R>) => {
      if (settled) return;
      settled = true;
      if (gen !== generation) return;
      stopTimer();
      active = null;
      ctrl = undefined;
      try {
        o.onSettled(code, result);
      } catch (e) {
        // A broken result handler must not stop the codes behind it.
        console.error('scan queue: result handler failed', e);
      }
      next();
    };
    emit();
    o.onStart?.(code);
    if (timeoutMs > 0) {
      timer = setTimeout(() => {
        timer = undefined;
        abort.abort();
        finish({ ok: false, error: new ScanTimeoutError() });
      }, timeoutMs);
    }
    let p: Promise<R>;
    try {
      p = Promise.resolve(o.run(code, abort.signal));
    } catch (e) {
      p = Promise.reject(e);
    }
    p.then(
      (value) => finish({ ok: true, value }),
      (error) => finish({ ok: false, error }),
    );
  };

  return {
    push(raw, opts = {}) {
      const code = raw.trim();
      if (!code) return 'empty';
      const t = now();
      if (opts.repeat !== false) {
        if (code === last.code && t - last.at < repeatMs) return 'repeat';
        // The camera keeps seeing a code while its request is slow: not a new scan.
        if (code === active || waiting.includes(code)) return 'repeat';
      }
      if (waiting.length >= maxPending) return 'full';
      last = { code, at: t };
      waiting.push(code);
      if (active === null) next();
      else emit();
      return 'queued';
    },
    clear() {
      generation += 1;
      stopTimer();
      ctrl?.abort();
      ctrl = undefined;
      active = null;
      waiting.length = 0;
      last = { code: '', at: 0 };
      emit();
    },
    resetRepeat() {
      last = { code: '', at: 0 };
    },
    holdTimeout: stopTimer,
    state,
  };
}
