"use client";

import type { ReactNode } from "react";

import { Card, CardContent } from "@/components/common/card";
import { cn } from "@/lib/utils";

type EntityHeaderProps = {
  title: ReactNode;
  /** Status chips, badges, etc. next to the title */
  badges?: ReactNode;
  /** Secondary line under the title (slug, email, etc.) */
  subtitle?: ReactNode;
  /** Optional description / body under subtitle */
  description?: ReactNode;
  /** Leading media (avatar, icon) */
  leading?: ReactNode;
  /** Extra content below the title block (e.g. summary field grid) */
  children?: ReactNode;
  className?: string;
};

/**
 * Detail-page hero card — shared by Users, Roles, and future modules.
 */
export function EntityHeader({
  title,
  badges,
  subtitle,
  description,
  leading,
  children,
  className,
}: EntityHeaderProps) {
  return (
    <Card className={cn("shadow-none", className)}>
      <CardContent
        className={cn(
          "flex flex-col gap-6 p-6",
          leading ? "sm:flex-row sm:items-center" : undefined,
        )}
      >
        {leading}
        <div className="min-w-0 flex-1 space-y-3">
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="font-display text-xl font-semibold tracking-tight">
              {title}
            </h2>
            {badges}
          </div>
          {subtitle ? (
            <div className="text-muted-foreground text-sm">{subtitle}</div>
          ) : null}
          {description ? <div className="text-sm">{description}</div> : null}
          {children}
        </div>
      </CardContent>
    </Card>
  );
}
