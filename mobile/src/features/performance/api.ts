import { unwrap } from '@/core/api/client';

type OptionalEnvelope<T> = Promise<{ data?: { data?: T }; error?: unknown; response: Response }>;

/**
 * The performance envelopes declare `data` as optional in the OpenAPI document;
 * the API always sends it on success, so this reads it like any other call.
 */
export function unwrapPerformance<T>(call: OptionalEnvelope<T>): Promise<T> {
  return unwrap(call as Promise<{ data?: { data: T }; error?: unknown; response: Response }>);
}
