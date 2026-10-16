import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { MOBILE_API_VERSION } from '@/core/config';

import { deviceInfo } from './device';

/** The session the app works with (`Me`); the mobile extras (device session) are not used yet. */
export type MobileMe = components['schemas']['Me'];
/** What the login call returns (tokens + session), as the client types it. */
export type MobileLoginResult = Awaited<ReturnType<typeof login>>;

/** The header is also set by the client middleware; the typed params require it. */
const params = { header: { 'X-Mobile-Api-Version': MOBILE_API_VERSION } };

export async function login(input: { email: string; password: string; totpCode?: string }) {
  return unwrap(
    api.POST('/v1/mobile/auth/login', {
      params,
      body: {
        email: input.email.trim(),
        password: input.password,
        totp_code: input.totpCode || undefined,
        device: await deviceInfo(),
      },
    }),
  );
}

export function fetchMe() {
  return unwrap(api.GET('/v1/mobile/auth/me', { params }));
}

export function logout() {
  return unwrap(api.POST('/v1/mobile/auth/logout', { params }));
}

export function switchOrganization(slug: string) {
  return unwrap(api.POST('/v1/mobile/auth/organization-context', { params, body: { organization_slug: slug } }));
}
