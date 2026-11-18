"use client";

import { useEffect, type ReactNode } from "react";

import { Toaster } from "@/components/ui/sonner";
import { setGlobalApiErrorHandler, type ApiError } from "@/lib/api";
import { toast } from "@/lib/toast";
import { useLocale } from "@/providers/locale-provider";

/**
 * Toast alanını kurar ve global API hata işleyicisini bağlar: her API hatası
 * `toast.apiError` ile kod → kullanıcı metni eşlemesinden geçer (TEC-1235).
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const { t } = useLocale();

  useEffect(() => {
    setGlobalApiErrorHandler((error: ApiError) => {
      toast.apiError(error, t);
    });
    return () => setGlobalApiErrorHandler(null);
  }, [t]);

  return (
    <>
      {children}
      <Toaster />
    </>
  );
}

/** Geriye dönük uyumluluk; yeni kodda `@/lib/toast` kullanın. */
export { appToast } from "@/lib/toast";
