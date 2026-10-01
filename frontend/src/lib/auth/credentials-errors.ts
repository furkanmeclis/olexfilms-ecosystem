import { CredentialsSignin } from "next-auth";

export const CREDENTIAL_ERROR_CODES = {
  MFA_REQUIRED: "MFA_REQUIRED",
  INVALID_MFA_CODE: "INVALID_MFA_CODE",
  MFA_NOT_ENROLLED: "MFA_NOT_ENROLLED",
  NO_TENANT_MEMBERSHIP: "NO_TENANT_MEMBERSHIP",
  ORGANIZATION_ACCESS_EXPIRED: "ORGANIZATION_ACCESS_EXPIRED",
  NO_PANEL_ACCESS: "NO_PANEL_ACCESS",
  NO_PORTAL_ACCESS: "NO_PORTAL_ACCESS",
  INVALID_OTP_CODE: "INVALID_OTP_CODE",
  OTP_LOCKED: "OTP_LOCKED",
} as const;

export type CredentialErrorCode =
  (typeof CREDENTIAL_ERROR_CODES)[keyof typeof CREDENTIAL_ERROR_CODES];

export class MFARequiredError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.MFA_REQUIRED;
}

export class InvalidMFACodeError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.INVALID_MFA_CODE;
}

export class MFANotEnrolledError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.MFA_NOT_ENROLLED;
}

export class NoTenantMembershipError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.NO_TENANT_MEMBERSHIP;
}

export class OrganizationAccessExpiredError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.ORGANIZATION_ACCESS_EXPIRED;
}

/** Customer / fleet only account on the panel login (TEC-90). */
export class NoPanelAccessError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.NO_PANEL_ACCESS;
}

/** Staff account on the portal login (TEC-90). */
export class NoPortalAccessError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.NO_PORTAL_ACCESS;
}

export class InvalidOTPCodeError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.INVALID_OTP_CODE;
}

export class OTPLockedError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.OTP_LOCKED;
}

export type CredentialSignInResult = {
  error?: string | null;
  code?: string | null;
  ok?: boolean;
  status?: number;
  url?: string | null;
};

export function resolveCredentialErrorCode(
  result: CredentialSignInResult,
): string | null {
  const code = result.code?.trim();
  if (code) return code;

  const error = result.error?.trim();
  if (!error) return null;

  const known = Object.values(CREDENTIAL_ERROR_CODES);
  for (const item of known) {
    if (error.includes(item)) return item;
  }
  return error;
}
