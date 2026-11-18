import { isApiError, type ApiError } from "@/lib/api";

export type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export type ApiErrorToast = {
  message: string;
  description?: string;
  /** Sabit id: aynı hata art arda gelirse tek toast kalır. */
  id?: string;
};

/** Sunucu mesajı teknik olan kodlar; yerelleştirilmiş metin gösterilir. */
const LOCALIZED_ERROR_CODES: Record<string, string> = {
  OTP_CHANNEL_UNAVAILABLE: "contracts.otp.errors.channel_unavailable",
  INVALID_OTP_CODE: "contracts.otp.errors.invalid_code",
  OTP_REQUIRED: "contracts.otp.errors.required",
};

/**
 * Durum koduyla zaten karşılanan genel kodlar: bunlar için `errors.codes.*`
 * yerine aşağıdaki durum dalları (ve doğrulamada sunucu metni) kullanılır.
 */
const GENERIC_CODES = new Set([
  "VALIDATION_ERROR",
  "UNAUTHENTICATED",
  "UNAUTHORIZED",
  "FORBIDDEN",
  "NOT_FOUND",
  "INTERNAL_ERROR",
]);

/** Ayrı ekranla ele alınan kodlar (LimitReachedDialog): toast yok. */
const SILENT_CODES = new Set(["LIMIT_REACHED", "SUBSCRIPTION_READ_ONLY"]);

function translated(t: Translate, key: string): string | undefined {
  const v = t(key);
  return v && v !== key ? v : undefined;
}

/**
 * API hatasını toast metnine çevirir: ne oldu (mesaj) ve gerekiyorsa
 * destek için başvuru numarası (açıklama). Teknik kod gösterilmez.
 * `null`: bu hata için toast gösterilmez.
 */
export function apiErrorToast(
  error: unknown,
  t: Translate,
): ApiErrorToast | null {
  if (!isApiError(error)) {
    return { message: t("common.error_generic") };
  }
  const e: ApiError = error;
  if (SILENT_CODES.has(e.code)) return null;
  if (e.code === "FEATURE_DISABLED") {
    return { message: t("errors.feature_disabled"), id: "feature-disabled" };
  }
  const localizedKey = LOCALIZED_ERROR_CODES[e.code];
  if (localizedKey) return { message: t(localizedKey) };
  if (e.code && !GENERIC_CODES.has(e.code)) {
    const byCode = translated(t, `errors.codes.${e.code}`);
    if (byCode) return { message: byCode };
  }
  if (e.isUnauthorized) {
    return { message: t("auth.session_expired"), id: "session-expired" };
  }
  if (e.isForbidden) return { message: t("common.error_forbidden") };
  if (e.isNotFound) return { message: t("common.error_not_found") };
  if (e.isValidation) {
    return { message: e.message || t("common.error_validation") };
  }
  if (e.isServer) {
    return {
      message: t("common.error_server"),
      ...(e.requestId
        ? { description: t("common.error_reference", { id: e.requestId }) }
        : {}),
    };
  }
  return { message: e.message || t("common.error_generic") };
}
