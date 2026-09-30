"use client";

import type { ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { cn } from "@/lib/utils";

type EntityUnavailableProps = {
  title: string;
  description: string;
  /** Optional footer note below the empty state (e.g. audit hint) */
  footer?: ReactNode;
  className?: string;
};

/**
 * Placeholder panel for detail sections blocked by API / OpenAPI gaps.
 */
export function EntityUnavailable({
  title,
  description,
  footer,
  className,
}: EntityUnavailableProps) {
  return (
    <div className={cn("space-y-4", className)}>
      <EmptyState title={title} description={description} className="py-10" />
      {footer}
    </div>
  );
}
