import { ApiError, toApiError } from '@/core/api/errors';
import { panelStream } from '@/core/api/client';

import type { LegalText, StreamEvent } from './chat-state';
import { createSseParser, createUtf8Decoder } from './sse';

/** A request refused before the stream started (non-2xx answer). */
export class AssistantRequestError extends ApiError {
  /** `data.consent` of a 428 AI_CONSENT_REQUIRED answer. */
  readonly consent: LegalText | null;
  constructor(base: ApiError, consent: LegalText | null) {
    super(base.status, { code: base.code, message: base.message, details: base.details }, base.requestId);
    this.consent = consent;
  }
}

/**
 * Runs one streamed turn (`/v1/ai/conversations/{uuid}/messages`,
 * `/v1/ai/actions/{uuid}/confirm|cancel`) and calls `onEvent` for every
 * event in order. Reads the `expo/fetch` body stream chunk by chunk; when
 * the platform gives no stream (e.g. EXPO_PUBLIC_USE_RN_FETCH) it falls back
 * to the whole body as text, parsed the same way. Resolves when the stream
 * ends; an aborted request rejects.
 */
export async function streamTurn(
  path: string,
  body: unknown,
  signal: AbortSignal,
  onEvent: (event: StreamEvent) => void,
): Promise<void> {
  let res: Awaited<ReturnType<typeof panelStream>>;
  try {
    res = await panelStream(path, body, signal);
  } catch (e) {
    if (signal.aborted) throw e;
    throw new ApiError(0, { code: 'NETWORK_ERROR', message: e instanceof Error ? e.message : String(e) });
  }
  if (!res.ok) {
    const payload = (await res.json().catch(() => undefined)) as { data?: { consent?: LegalText } } | undefined;
    throw new AssistantRequestError(toApiError(res.status, payload), payload?.data?.consent ?? null);
  }
  const parser = createSseParser((e) => onEvent(e as StreamEvent));
  const stream = res.body;
  if (!stream) {
    parser.push(await res.text());
    parser.end();
    return;
  }
  const reader = stream.getReader();
  const decoder = createUtf8Decoder();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (value) parser.push(decoder.decode(value));
      if (done) break;
    }
    parser.push(decoder.decode());
    parser.end();
  } finally {
    reader.releaseLock();
  }
}
