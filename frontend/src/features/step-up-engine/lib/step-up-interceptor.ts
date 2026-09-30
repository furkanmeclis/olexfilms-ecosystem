import { ApiError } from "@/lib/api/errors";

type StepUpEnsure = () => Promise<boolean>;

let ensureStepUp: StepUpEnsure | null = null;

export function registerStepUpEnsure(fn: StepUpEnsure | null) {
  ensureStepUp = fn;
}

export function unregisterStepUpEnsure() {
  ensureStepUp = null;
}

export async function runStepUpEnsure(): Promise<boolean> {
  if (!ensureStepUp) {
    return false;
  }
  return ensureStepUp();
}

export function isStepUpRequired(error: unknown): error is ApiError {
  return error instanceof ApiError && error.code === "STEP_UP_REQUIRED";
}

export async function withStepUpRetry<T>(run: () => Promise<T>): Promise<T> {
  try {
    return await run();
  } catch (error) {
    if (!isStepUpRequired(error) || !ensureStepUp) {
      throw error;
    }
    const ok = await ensureStepUp();
    if (!ok) {
      throw error;
    }
    return run();
  }
}
