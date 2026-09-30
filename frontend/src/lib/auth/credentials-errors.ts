import { CredentialsSignin } from "next-auth";

export const CREDENTIAL_ERROR_CODES = {
  MFA_REQUIRED: "MFA_REQUIRED",
  INVALID_MFA_CODE: "INVALID_MFA_CODE",
  MFA_NOT_ENROLLED: "MFA_NOT_ENROLLED",
  NO_TENANT_MEMBERSHIP: "NO_TENANT_MEMBERSHIP",
  ORGANIZATION_ACCESS_EXPIRED: "ORGANIZATION_ACCESS_EXPIRED",
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
