"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

type EntityFormProps = {
  children: ReactNode;
  className?: string;
};

/**
 * Consistent outer shell for create/edit form pages under EntityPage.
 * Feature forms still own AppForm + FormLayout internals.
 */
export function EntityForm({ children, className }: EntityFormProps) {
  return (
    <div className={cn("mx-auto w-full max-w-5xl", className)}>{children}</div>
  );
}
