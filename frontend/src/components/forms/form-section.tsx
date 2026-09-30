"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

type FormSectionProps = {
  id?: string;
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
  /** Grid columns for fields inside the section */
  columns?: 1 | 2;
};

export function FormSection({
  id,
  title,
  description,
  children,
  className,
  columns = 1,
}: FormSectionProps) {
  return (
    <section
      id={id}
      className={cn(
        "border-border bg-card text-card-foreground scroll-mt-24 rounded-xl border",
        className,
      )}
    >
      <header className="border-border border-b px-5 py-4 sm:px-6">
        <h2 className="font-display text-base font-semibold tracking-tight">
          {title}
        </h2>
        {description ? (
          <p className="text-muted-foreground mt-1 text-sm">{description}</p>
        ) : null}
      </header>
      <div
        className={cn(
          "grid min-w-0 gap-5 p-5 sm:p-6",
          columns === 2 && "sm:grid-cols-2",
        )}
      >
        {children}
      </div>
    </section>
  );
}
