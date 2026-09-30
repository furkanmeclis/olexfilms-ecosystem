"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

type FormActionsProps = {
  children: ReactNode;
  className?: string;
  sticky?: boolean;
};

/** Bottom action bar for form pages (cancel / submit). */
export function FormActions({
  children,
  className,
  sticky = true,
}: FormActionsProps) {
  return (
    <div
      className={cn(
        "border-border bg-card flex flex-wrap items-center justify-end gap-2 rounded-xl border px-4 py-3",
        sticky &&
          "supports-[backdrop-filter]:bg-card/95 sticky bottom-3 z-10 shadow-sm supports-[backdrop-filter]:backdrop-blur",
        className,
      )}
    >
      {children}
    </div>
  );
}
