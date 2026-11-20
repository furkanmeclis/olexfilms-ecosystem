/**
 * TEC-1257: public legal texts, the consent prompt after sign-in and the
 * account deletion. Pure helpers (node:test), no React Native imports.
 */

/** Public legal texts (GET /v1/public/legal-texts/{kind}), in link order. */
export const LEGAL_KINDS = ['privacy_policy', 'kvkk_notice', 'terms_of_use'] as const;
export type LegalKind = (typeof LEGAL_KINDS)[number];

export function isLegalKind(value: unknown): value is LegalKind {
  return typeof value === 'string' && (LEGAL_KINDS as readonly string[]).includes(value);
}

/** URL of a public legal text; the server falls back to tr, then en. */
export function legalTextUrl(apiUrl: string, kind: LegalKind, locale: string): string {
  return `${apiUrl.replace(/\/+$/, '')}/v1/public/legal-texts/${kind}?locale=${encodeURIComponent(locale)}`;
}

/** The one text the app asks after sign-in (the web portal's AIConsentDialog). */
export const CONSENT_KIND = 'ai_guidelines';

export type PendingText = { kind: string; locale: string; version: number; body: string };

/** The pending text to show, or null (only the AI guidelines are asked; K22). */
export function pendingConsent<T extends PendingText>(items: readonly T[] | null | undefined): T | null {
  return items?.find((i) => i.kind === CONSENT_KIND && i.version >= 1) ?? null;
}

/** Message key (legal namespace) of a failed deletion step, by API error code. */
export function deleteErrorKey(code: string | null | undefined): string {
  switch (code) {
    case 'INVALID_OTP_CODE':
      return 'invalid_code';
    case 'OTP_LOCKED':
      return 'otp_locked';
    case 'RATE_LIMITED':
      return 'rate_limited';
    case 'OTP_DELIVERY_FAILED':
    case 'RATE_LIMITER_UNAVAILABLE':
      return 'delivery_failed';
    case 'ACCOUNT_HAS_NO_PHONE':
      return 'no_phone';
    case 'CUSTOMER_HAS_PANEL_ACCOUNT':
      return 'panel_account';
    default:
      return 'failed';
  }
}

/** A wrong or expired code keeps the code step open; other errors start over. */
export function keepsCodeStep(code: string | null | undefined): boolean {
  return code === 'INVALID_OTP_CODE';
}
