"use client";

import type { ReactNode } from "react";

import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";

export type EntityDetailField = {
  key: string;
  label: string;
  value: ReactNode;
};

export type EntityDetailSection = {
  id: string;
  /** Optional when the parent surface already provides a heading (e.g. CardTitle). */
  title?: string;
  description?: string;
  fields?: EntityDetailField[];
  children?: ReactNode;
};

type EntityDetailProps = {
  sections: EntityDetailSection[];
  className?: string;
};

/**
 * Read-only detail layout for entity detail pages (and legacy drawers).
 */
export function EntityDetail({ sections, className }: EntityDetailProps) {
  return (
    <div className={cn("space-y-6", className)}>
      {sections.map((section, index) => (
        <section
          key={section.id}
          aria-labelledby={
            section.title ? `entity-detail-${section.id}` : undefined
          }
        >
          {index > 0 ? <Separator className="mb-6" /> : null}
          {section.title || section.description ? (
            <div className="mb-3 space-y-1">
              {section.title ? (
                <h3
                  id={`entity-detail-${section.id}`}
                  className="text-sm font-semibold tracking-tight"
                >
                  {section.title}
                </h3>
              ) : null}
              {section.description ? (
                <p className="text-muted-foreground text-xs">
                  {section.description}
                </p>
              ) : null}
            </div>
          ) : null}
          {section.fields?.length ? (
            <dl className="grid gap-3 sm:grid-cols-2">
              {section.fields.map((field) => (
                <div key={field.key} className="space-y-1">
                  <dt className="text-muted-foreground text-xs font-medium">
                    {field.label}
                  </dt>
                  <dd className="text-sm break-words">{field.value ?? "—"}</dd>
                </div>
              ))}
            </dl>
          ) : null}
          {section.children}
        </section>
      ))}
    </div>
  );
}
