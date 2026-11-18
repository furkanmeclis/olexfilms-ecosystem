/**
 * Ortak toast yardımcısı (TEC-1235). Uygulamadaki tüm anlık bildirimler
 * buradan geçer; `sonner` doğrudan içe aktarılmaz (ESLint kuralı).
 *
 * - Durumlar: success, error, warning, info, loading (+ nötr `message`).
 * - Süreler: başarı/bilgi 4 sn, uyarı 5 sn, hata 6 sn, yükleniyor kalıcı.
 * - Tekrar bastırma: aynı tür + aynı metin görünürken yeni toast açılmaz,
 *   mevcut toast yenilenir (sonner'da aynı `id` güncelleme demektir).
 * - Aksiyon: `{ action: { label, onClick } }` (Geri al, Tekrar dene...).
 * - API hataları: `toast.apiError(error, t)` kod → kullanıcı metni eşler.
 */
import type { ReactNode } from "react";
import { toast as sonner, type ExternalToast } from "sonner";

import { apiErrorToast, type Translate } from "./api-error";

export { apiErrorToast, type Translate } from "./api-error";

export type ToastKind = "success" | "error" | "warning" | "info";

export type ToastOptions = Pick<
  ExternalToast,
  | "id"
  | "description"
  | "duration"
  | "action"
  | "cancel"
  | "onDismiss"
  | "onAutoClose"
  | "dismissible"
  | "icon"
> & {
  /** false: aynı metin için ikinci bir toast açılabilir. Varsayılan true. */
  dedupe?: boolean;
};

export const TOAST_DURATIONS: Record<ToastKind, number> = {
  success: 4000,
  info: 4000,
  warning: 5000,
  error: 6000,
};

/** Tekrar bastırma anahtarı: yalnızca düz metin mesajlar için. */
export function dedupeId(kind: string, message: ReactNode): string | undefined {
  return typeof message === "string" && message.trim() !== ""
    ? `${kind}:${message}`
    : undefined;
}

function show(kind: ToastKind, message: ReactNode, opts: ToastOptions = {}) {
  const { dedupe = true, ...rest } = opts;
  const id = rest.id ?? (dedupe ? dedupeId(kind, message) : undefined);
  return sonner[kind](message, {
    duration: TOAST_DURATIONS[kind],
    ...rest,
    ...(id !== undefined ? { id } : {}),
  });
}

export const toast = {
  success: (message: ReactNode, opts?: ToastOptions) =>
    show("success", message, opts),
  error: (message: ReactNode, opts?: ToastOptions) =>
    show("error", message, opts),
  warning: (message: ReactNode, opts?: ToastOptions) =>
    show("warning", message, opts),
  info: (message: ReactNode, opts?: ToastOptions) =>
    show("info", message, opts),
  /** Nötr bilgi (ikon yok). */
  message: (message: ReactNode, opts?: ToastOptions) => {
    const { dedupe = true, ...rest } = opts ?? {};
    const id = rest.id ?? (dedupe ? dedupeId("message", message) : undefined);
    return sonner.message(message, {
      duration: TOAST_DURATIONS.info,
      ...rest,
      ...(id !== undefined ? { id } : {}),
    });
  },
  /** Süren işlem; dönen id ile `toast.success(msg, { id })` sonucu yazar. */
  loading: (message: ReactNode, opts?: Omit<ToastOptions, "dedupe">) =>
    sonner.loading(message, opts),
  promise: sonner.promise,
  dismiss: (id?: string | number) => sonner.dismiss(id),
  /** API hatasını kullanıcı metnine çevirip hata toast'ı gösterir. */
  apiError: (error: unknown, t: Translate, opts?: ToastOptions) => {
    const r = apiErrorToast(error, t);
    if (!r) return undefined;
    return show("error", r.message, {
      ...(r.description ? { description: r.description } : {}),
      ...(r.id ? { id: r.id } : {}),
      ...opts,
    });
  },
};

/**
 * Eski çağrı biçimi (`appToast.success(msg)`); `toast` ile aynı davranır.
 * Yeni kodda `toast` kullanın.
 */
export const appToast = {
  success: (message: string, opts?: ToastOptions) =>
    toast.success(message, opts),
  error: (message: string, opts?: ToastOptions) => toast.error(message, opts),
  warning: (message: string, opts?: ToastOptions) =>
    toast.warning(message, opts),
  info: (message: string, opts?: ToastOptions) => toast.info(message, opts),
};
