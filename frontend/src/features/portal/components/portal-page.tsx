"use client";

import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
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

/**
 * TEC-771: loading state of the portal pages: card shaped placeholders
 * plus the label for screen readers (and tests), instead of a bare line.
 */
export function PortalLoading({
  label,
  rows = 3,
  testId,
}: {
  label: string;
  rows?: number;
  testId?: string;
}) {
  return (
    <div role="status" aria-live="polite" data-testid={testId}>
      <span className="sr-only">{label}</span>
      <div className="space-y-3" aria-hidden>
        {Array.from({ length: rows }, (_, i) => (
          <div key={i} className="bg-card space-y-3 rounded-xl border p-5">
            <Skeleton className="h-4 w-2/5" />
            <Skeleton className="h-3 w-3/5" />
          </div>
        ))}
      </div>
    </div>
  );
}

/** TEC-771: empty state card shared by the portal lists. */
export function PortalEmpty({
  icon,
  title,
  description,
  action,
  testId,
}: {
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  testId?: string;
}) {
  return (
    <Card>
      <CardContent
        className="flex flex-col items-center gap-2 py-10 text-center"
        data-testid={testId}
      >
        {icon ? (
          <span className="text-muted-foreground [&_svg]:size-10" aria-hidden>
            {icon}
          </span>
        ) : null}
        <p className="font-medium">{title}</p>
        {description ? (
          <p className="text-muted-foreground max-w-sm text-sm">
            {description}
          </p>
        ) : null}
        {action ? <div className="mt-2">{action}</div> : null}
      </CardContent>
    </Card>
  );
}
