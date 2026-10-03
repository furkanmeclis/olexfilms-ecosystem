import { isApiError } from "@/lib/api";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Glorian admin error codes (TEC-273) with a text of their own. */
const CODE_KEYS: Record<string, string> = {
  GLORIAN_CONNECTION_INACTIVE: "integrations.glorian.errors.inactive",
  GLORIAN_OUTBOUND_NOT_REPLAYABLE: "integrations.glorian.errors.not_replayable",
  GLORIAN_CENTER_MISSING: "integrations.glorian.errors.center_missing",
  QUEUE_UNAVAILABLE: "integrations.glorian.errors.queue_unavailable",
  VALIDATION_ERROR: "integrations.glorian.errors.validation",
};

/** Toast text of a failed Glorian call. */
export function glorianErrorText(t: Translate, error: unknown): string {
  if (isApiError(error)) {
    const key = CODE_KEYS[error.code];
    if (key) return t(key);
    if (error.status === 404) return t("integrations.glorian.errors.not_found");
    if (error.message) return error.message;
  }
  return t("integrations.glorian.errors.failed");
}

/** 404 of a list call: the connection is not saved yet. */
export function isNotConfigured(error: unknown): boolean {
  return isApiError(error) && error.status === 404;
}
