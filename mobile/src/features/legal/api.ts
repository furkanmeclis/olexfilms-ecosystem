import { ApiError } from '@/core/api/errors';
import { API_URL, APP_VERSION } from '@/core/config';

import { type LegalKind, legalTextUrl } from './lib';

export type LegalText = { kind: string; locale: string; version: number; body: string; created_at: string };

/**
 * A public legal text (GET /v1/public/legal-texts/{kind}): no token and not
 * through the panel / portal mirror, so the sign-in screen can open it.
 * 404 means nothing is published yet (ApiError status 404).
 */
export async function fetchLegalText(kind: LegalKind, locale: string, signal?: AbortSignal): Promise<LegalText> {
  let res: Response;
  try {
    res = await fetch(legalTextUrl(API_URL, kind, locale), {
      signal,
      headers: { Accept: 'application/json', 'Accept-Language': locale, 'X-App-Version': APP_VERSION },
    });
  } catch {
    throw new ApiError(0);
  }
  if (!res.ok) throw new ApiError(res.status);
  const body = (await res.json()) as { data?: LegalText };
  if (!body.data) throw new ApiError(500);
  return body.data;
}
