"use client";

import type { ReactNode } from "react";
import Link from "next/link";

import { AppWordmark } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

export function AuthShell({ children }: { children: ReactNode }) {
  const { t } = useLocale();

  return (
    <div className="from-background via-muted/70 to-accent/25 relative flex min-h-svh flex-col items-center justify-center bg-gradient-to-br p-6 md:p-10">
      <div className="absolute top-4 right-4 flex items-center gap-1">
        <LocaleSwitch />
        <ThemeSwitch />
      </div>
      <div className="flex w-full max-w-sm flex-col gap-6">
        <Link
          href={routes.guest.login}
          className="self-center"
          aria-label={t("common.app_product")}
        >
          <AppWordmark />
        </Link>
        {children}
      </div>
    </div>
  );
}
