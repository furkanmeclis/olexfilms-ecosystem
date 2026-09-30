"use client";

import type { ReactNode } from "react";

import { StepUpProvider } from "@/features/step-up-engine";
import { AuthProvider } from "@/providers/auth-provider";
import { LocaleHydrator } from "@/components/layout/locale-hydrator";
import { DialogProvider } from "@/providers/dialog-provider";
import { LocaleProvider } from "@/providers/locale-provider";
import { PermissionProvider } from "@/providers/permission-provider";
import { QueryProvider } from "@/providers/query-provider";
import { RealtimeProvider } from "@/providers/realtime-provider";
import { ThemeProvider } from "@/providers/theme-provider";
import { ToastProvider } from "@/providers/toast-provider";

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <LocaleProvider>
      <ThemeProvider>
        <QueryProvider>
          <AuthProvider>
            <LocaleHydrator />
            <PermissionProvider>
              <StepUpProvider>
                <ToastProvider>
                  <RealtimeProvider>
                    <DialogProvider>{children}</DialogProvider>
                  </RealtimeProvider>
                </ToastProvider>
              </StepUpProvider>
            </PermissionProvider>
          </AuthProvider>
        </QueryProvider>
      </ThemeProvider>
    </LocaleProvider>
  );
}
