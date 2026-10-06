import { isApiError } from "@/lib/api";

/**
 * Where a signing error is shown (TEC-291): next to the OTP button, under
 * the code input, under the signature canvas, or as a generic message.
 */
export type SignErrorField = "otp" | "code" | "signature" | null;

export type SignError = { field: SignErrorField; key: string };

const KEY = "services.contract.errors";

/**
 * Error of "OTP gönder". 4xx answers are shown at the OTP field; a 500 (the
 * contracts OTP endpoint still answers 500 on rate limit or delivery
 * failure) or a network error stays generic.
 */
export function otpRequestError(error: unknown): SignError {
  if (!isApiError(error) || error.status >= 500) {
    return { field: null, key: `${KEY}.otp_failed` };
  }
  if (error.status === 429) return { field: "otp", key: `${KEY}.otp_rate` };
  if (error.status === 409) {
    return { field: "otp", key: `${KEY}.already_signed` };
  }
  if (error.status === 400) return { field: "otp", key: `${KEY}.no_phone` };
  return { field: "otp", key: `${KEY}.otp_failed` };
}

/**
 * Error of a sign call. A wrong / expired code (400 "invalid OTP code", 422
 * CONTRACT_SIGN_WINDOW_EXPIRED) goes under the code input, any other 400
 * under the signature, 409 and 5xx are generic.
 */
export function signRequestError(error: unknown): SignError {
  if (!isApiError(error) || error.status >= 500) {
    return { field: null, key: `${KEY}.generic` };
  }
  if (error.code === "CONTRACT_SIGN_WINDOW_EXPIRED") {
    return { field: "code", key: `${KEY}.window_expired` };
  }
  if (error.status === 409) {
    return { field: null, key: `${KEY}.already_signed` };
  }
  if (error.status === 400) {
    const otp =
      /otp/i.test(error.message) ||
      error.details.some((d) => d.field === "code");
    return otp
      ? { field: "code", key: `${KEY}.invalid_code` }
      : { field: "signature", key: `${KEY}.invalid_signature` };
  }
  return { field: null, key: `${KEY}.generic` };
}

/** "mm:ss" of a remaining time (never negative). */
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}
