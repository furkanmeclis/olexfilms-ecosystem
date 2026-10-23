import { api, unwrap } from '@/core/api/client';

export type StepUpMethod = 'password' | 'totp';

/**
 * Earns the per-user step-up grant through the mobile mirror (TEC-961): the
 * same password / authenticator check, rate limit and audit as the web
 * dialog. Passkey step-up stays on the web panel.
 */
export function verifyStepUp(method: StepUpMethod, secret: string) {
  return method === 'password'
    ? unwrap(api.POST('/v1/auth/step-up/password', { body: { password: secret } }))
    : unwrap(api.POST('/v1/auth/step-up/totp', { body: { code: secret } }));
}
