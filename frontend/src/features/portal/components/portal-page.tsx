"use client";

import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/**
 * Frame of the TEC-241 portal pages: language and direction on the page
 * root (Arabic is right-to-left), a title row with an optional back link.
 */
export function PortalPage({
  title,
  icon,
  back,
  actions,
  testId,
  children,
}: {
  title: ReactNode;
  icon?: ReactNode;
  back?: { href: string; label: string };
  actions?: ReactNode;
  testId?: string;
  children: ReactNode;
}) {
  const { locale, dir } = useLocale();
  return (
    <div
      dir={dir}
      lang={locale}
      data-testid={testId}
      className="mx-auto w-full max-w-3xl space-y-6 px-4 py-8"
    >
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h1 className="flex min-w-0 items-center gap-2 text-xl font-semibold">
          {icon}
          <span className="truncate">{title}</span>
        </h1>
        <div className="flex flex-wrap items-center gap-2">
          {actions}
          {back ? (
            <Button asChild variant="outline" size="sm">
              <Link href={back.href}>
                <ArrowLeft className="size-4 rtl:rotate-180" />
                {back.label}
              </Link>
            </Button>
          ) : null}
        </div>
      </div>
      {children}
    </div>
  );
}
