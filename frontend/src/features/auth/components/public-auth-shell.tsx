"use client";

import type { ReactNode } from "react";
import Link from "next/link";

import { AppWordmark } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

export function PublicAuthShell({ children }: { children: ReactNode }) {
  const { t } = useLocale();

  return (
    <div className="bg-muted relative flex min-h-svh flex-col items-center justify-center p-6 md:p-10">
      <div className="absolute end-4 top-4 flex items-center gap-1">
        <LocaleSwitch />
        <ThemeSwitch />
      </div>
      <div className="flex w-full max-w-lg flex-col gap-6">
        <Link
          href={routes.public.root}
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
