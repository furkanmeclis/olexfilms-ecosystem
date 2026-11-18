import { i18n } from '@/core/i18n';

import { describeError, type ErrorTextOptions } from './error-text';
import { createToaster, type ToastOptions } from './store';

const toaster = createToaster();
const translator = { exists: (k: string) => i18n.exists(k), t: (k: string) => i18n.t(k) };

/** Uygulamanın tek toast deposu; `ToastHost` bunu çizer. */
export const toastStore = toaster.store;

type ApiErrorOptions = ToastOptions &
  ErrorTextOptions & {
    /** Verilirse başlık bu olur, hata metni açıklamaya iner ("Kaydedilemedi" + neden). */
    title?: string;
  };

/**
 * Kısa, engellemeyen bildirimler (TEC-1235). Onay/seçim isteyen diyaloglar
 * `Alert.alert(..., buttons)` / `confirmAction` olarak kalır.
 *
 * ```ts
 * toast.success(t('saved'));
 * const id = toast.loading(t('sending'));
 * toast.success(t('sent'), { id });
 * toast.apiError(e, { title: t('action_failed') });
 * ```
 */
export const toast = {
  success: toaster.success,
  error: toaster.error,
  warning: toaster.warning,
  info: toaster.info,
  loading: toaster.loading,
  promise: toaster.promise,
  dismiss: toaster.dismiss,
  /** Başarısız API çağrısı: kod, uygulamanın hata kataloğundan kullanıcı metnine çevrilir. */
  apiError(e: unknown, opts: ApiErrorOptions = {}) {
    const { title, namespaces, fallback, ...rest } = opts;
    const text = describeError(e, translator, { namespaces, fallback });
    return title ? toaster.error(title, { ...rest, description: text }) : toaster.error(text, rest);
  },
};
