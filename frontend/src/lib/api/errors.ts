import { i18nConfig, type AppLocale } from "@/config/i18n";
import { translate } from "@/lib/i18n/messages";

import {
  emitLimitEvent,
  isLimitEventCode,
  limitDetailFrom,
} from "./limit-events";

export type ApiErrorDetail = {
  field?: string;
  message?: string;
  code?: string;
};

export type ApiErrorBody = {
  success: false;
  error: {
    code: string;
    message: string;
    details?: ApiErrorDetail[];
  };
  meta?: {
    request_id?: string;
  };
};

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ApiErrorDetail[];
  readonly requestId?: string;
  readonly body?: ApiErrorBody;

  constructor(opts: {
    status: number;
    code: string;
    message: string;
    details?: ApiErrorDetail[];
    requestId?: string;
    body?: ApiErrorBody;
  }) {
    super(opts.message);
    this.name = "ApiError";
    this.status = opts.status;
    this.code = opts.code;
    this.details = opts.details ?? [];
    this.requestId = opts.requestId;
    this.body = opts.body;
  }

  get isUnauthorized() {
    return this.status === 401;
  }

  get isForbidden() {
    return this.status === 403;
  }

  get isNotFound() {
    return this.status === 404;
  }

  get isValidation() {
    return this.status === 422 || this.status === 400;
  }

  get isServer() {
    return this.status >= 500;
  }

  fieldErrors(): Record<string, string> {
    const map: Record<string, string> = {};
    for (const detail of this.details) {
      if (detail.field && detail.message) {
        map[detail.field] = detail.message;
      }
    }
    return map;
  }
}

export function parseApiError(
  status: number,
  data: unknown,
  options?: { emitLimitEvent?: boolean },
): ApiError {
  const body = data as Partial<ApiErrorBody> | null;
  const code = body?.error?.code ?? `HTTP_${status}`;
  const details = body?.error?.details ?? [];
  const message = localizedMessage(code, status, body?.error?.message);
  if (options?.emitLimitEvent !== false && isLimitEventCode(code)) {
    emitLimitEvent(limitDetailFrom(code, details));
  }
  return new ApiError({
    status,
    code,
    message,
    details,
    requestId: body?.meta?.request_id,
    body: body as ApiErrorBody | undefined,
  });
}

const TURKISH_CHARS = /[çğıöşüÇĞİÖŞÜ]/;

function activeLocale(): AppLocale {
  if (typeof document !== "undefined") {
    const lang = document.documentElement.lang;
    if (i18nConfig.supportedLocales.includes(lang as AppLocale))
      return lang as AppLocale;
  }
  return i18nConfig.defaultLocale;
}

/**
 * Keeps the server message when it is already in the active language
 * (some modules answer in Turkish); otherwise shows the localized text for
 * the error code, falling back to the server message, then to an HTTP text.
 */
function localizedMessage(
  code: string,
  status: number,
  serverMessage?: string,
) {
  const locale = activeLocale();
  const server = serverMessage?.trim();
  const serverIsTurkish = server ? TURKISH_CHARS.test(server) : false;
  if (server && (locale === "tr") === serverIsTurkish) return server;
  const key = `errors.codes.${code}`;
  const byCode = translate(locale, key);
  if (byCode !== key) return byCode;
  if (server) return server;
  const httpKey = `errors.http.${[401, 403, 404, 422, 500].includes(status) ? status : "default"}`;
  return translate(locale, httpKey);
}

export type ErrorHandler = (error: ApiError) => void;

let globalErrorHandler: ErrorHandler | null = null;

export function setGlobalApiErrorHandler(handler: ErrorHandler | null) {
  globalErrorHandler = handler;
}

export function emitApiError(error: ApiError) {
  globalErrorHandler?.(error);
}
