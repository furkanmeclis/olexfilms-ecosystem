import type { components } from './schema';

type ErrorBody = components['schemas']['ErrorBody'];

/** A failed API call, carrying the backend's machine-readable error code. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ErrorBody['details'];
  readonly requestId?: string;

  constructor(status: number, body?: Partial<ErrorBody>, requestId?: string) {
    super(body?.message ?? `HTTP ${status}`);
    this.name = 'ApiError';
    this.status = status;
    this.code = body?.code ?? (status === 0 ? 'NETWORK_ERROR' : 'INTERNAL_ERROR');
    this.details = body?.details;
    this.requestId = requestId;
  }

  /** Field-level validation messages keyed by field name. */
  fieldErrors(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const d of this.details ?? []) {
      if (d.field) out[d.field] = d.message ?? d.code ?? '';
    }
    return out;
  }
}

export function isApiError(e: unknown, code?: string): e is ApiError {
  return e instanceof ApiError && (code === undefined || e.code === code);
}

export function toApiError(status: number, payload: unknown): ApiError {
  const p = payload as { error?: Partial<ErrorBody>; meta?: { request_id?: string } } | undefined;
  return new ApiError(status, p?.error, p?.meta?.request_id);
}
