import { createStore } from 'zustand/vanilla';

/**
 * Toast durumu (TEC-1235). React ve react-native'e bağımlı değil: hem ekranlar
 * dışından (mutation callback'leri, yardımcı fonksiyonlar) çağrılabilir hem de
 * `node --test` ile doğrudan test edilir. Görünüm `toast-host.tsx`'te.
 */

export type ToastKind = 'success' | 'error' | 'warning' | 'info' | 'loading';

export type ToastAction = { label: string; onPress: () => void };

export type ToastOptions = {
  /** Aynı id ile yeni çağrı mevcut toast'ı yerinde değiştirir (loading → success). */
  id?: string;
  description?: string;
  /** Milisaniye; `Infinity` kapatılana kadar kalır. Varsayılan türe göre. */
  duration?: number;
  action?: ToastAction;
};

export type ToastItem = {
  id: string;
  kind: ToastKind;
  message: string;
  description?: string;
  action?: ToastAction;
  duration: number;
  /** Her gösterim/yenilemede artar; host ekran okuyucu duyurusunu buna bağlar. */
  version: number;
};

/** Web ile aynı süreler: başarı/bilgi 4 sn, uyarı 5 sn, hata 6 sn, yükleniyor değiştirilene kadar. */
export const DURATIONS: Record<ToastKind, number> = {
  success: 4000,
  info: 4000,
  warning: 5000,
  error: 6000,
  loading: Infinity,
};

/** Aynı anda en fazla bu kadar toast; en yenisi üstte, taşan en eski kapanır. */
export const MAX_TOASTS = 3;

export type ToastState = { toasts: ToastItem[] };

export function createToaster() {
  const store = createStore<ToastState>(() => ({ toasts: [] }));
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  let seq = 0;

  const clearTimer = (id: string) => {
    const t = timers.get(id);
    if (t !== undefined) clearTimeout(t);
    timers.delete(id);
  };

  const arm = (item: ToastItem) => {
    clearTimer(item.id);
    if (Number.isFinite(item.duration) && item.duration > 0) {
      timers.set(
        item.id,
        setTimeout(() => dismiss(item.id), item.duration),
      );
    }
  };

  function dismiss(id?: string) {
    if (id === undefined) {
      for (const k of [...timers.keys()]) clearTimer(k);
      store.setState({ toasts: [] });
      return;
    }
    clearTimer(id);
    const { toasts } = store.getState();
    if (toasts.some((t) => t.id === id)) store.setState({ toasts: toasts.filter((t) => t.id !== id) });
  }

  function show(kind: ToastKind, message: string, opts: ToastOptions = {}): string {
    const { toasts } = store.getState();
    const duration = opts.duration ?? DURATIONS[kind];

    // Aynı id: yerinde değiştir (yükleniyor → sonuç), sırası korunur.
    const byId = opts.id === undefined ? -1 : toasts.findIndex((t) => t.id === opts.id);
    if (byId >= 0) {
      const prev = toasts[byId];
      const next: ToastItem = {
        id: prev.id,
        kind,
        message,
        description: opts.description,
        action: opts.action,
        duration,
        version: prev.version + 1,
      };
      store.setState({ toasts: toasts.map((t, i) => (i === byId ? next : t)) });
      arm(next);
      return next.id;
    }

    // Görünürken aynı tür + metin: yığmak yerine süreyi yenile.
    const dup = toasts.find((t) => t.kind === kind && t.message === message && t.description === opts.description);
    if (dup) {
      const next: ToastItem = { ...dup, action: opts.action ?? dup.action, duration, version: dup.version + 1 };
      store.setState({ toasts: toasts.map((t) => (t === dup ? next : t)) });
      arm(next);
      return dup.id;
    }

    const item: ToastItem = {
      id: opts.id ?? `t${++seq}`,
      kind,
      message,
      description: opts.description,
      action: opts.action,
      duration,
      version: 1,
    };
    const all = [item, ...toasts];
    for (const gone of all.slice(MAX_TOASTS)) clearTimer(gone.id);
    store.setState({ toasts: all.slice(0, MAX_TOASTS) });
    arm(item);
    return item.id;
  }

  type Msg<T> = string | ((value: T) => string);
  const resolve = <T>(m: Msg<T>, v: T) => (typeof m === 'function' ? m(v) : m);

  /** Yükleniyor gösterir, sonuçlanınca aynı toast'ı başarı ya da hataya çevirir. */
  function promise<T>(p: Promise<T>, msgs: { loading: string; success: Msg<T>; error: Msg<unknown> }): Promise<T> {
    const id = show('loading', msgs.loading);
    p.then(
      (v) => show('success', resolve(msgs.success, v), { id }),
      (e: unknown) => show('error', resolve(msgs.error, e), { id }),
    );
    return p;
  }

  return {
    store,
    show,
    dismiss,
    promise,
    success: (message: string, opts?: ToastOptions) => show('success', message, opts),
    error: (message: string, opts?: ToastOptions) => show('error', message, opts),
    warning: (message: string, opts?: ToastOptions) => show('warning', message, opts),
    info: (message: string, opts?: ToastOptions) => show('info', message, opts),
    loading: (message: string, opts?: ToastOptions) => show('loading', message, opts),
  };
}

export type Toaster = ReturnType<typeof createToaster>;
