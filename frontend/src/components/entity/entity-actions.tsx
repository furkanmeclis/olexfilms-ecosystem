"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

type EntityActionsProps = {
  children: ReactNode;
  className?: string;
};

/**
 * Standard action toolbar for entity detail / list header actions.
 */
export function EntityActions({ children, className }: EntityActionsProps) {
  return (
    <div className={cn("flex flex-wrap items-center gap-2", className)}>
      {children}
    </div>
  );
}
