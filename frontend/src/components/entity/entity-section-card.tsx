"use client";

import type { ReactNode } from "react";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type EntitySectionCardProps = {
  title: string;
  children: ReactNode;
  className?: string;
  contentClassName?: string;
  /** When true, the whole card body can be collapsed via the header. */
  collapsible?: boolean;
  /** Initial open state when `collapsible` is true. Default: true. */
  defaultOpen?: boolean;
  /** Optional count badge next to the title (e.g. permission total). */
  badge?: string | number;
  /** Optional action element rendered to the right of the title (e.g. a button). */
  action?: ReactNode;
};

/**
 * Titled card section used on entity detail pages.
 */
export function EntitySectionCard({
  title,
  children,
  className,
  contentClassName,
  collapsible = false,
  defaultOpen = true,
  badge,
  action,
}: EntitySectionCardProps) {
  if (!collapsible) {
    return (
      <Card className={cn("shadow-none", className)}>
        <CardHeader>
          <div className="flex items-center justify-between gap-2">
            <CardTitle className="flex items-center gap-2">
              <span>{title}</span>
              {badge != null ? (
                <Badge variant="secondary" className="font-normal tabular-nums">
                  {badge}
                </Badge>
              ) : null}
            </CardTitle>
            {action ? <div className="shrink-0">{action}</div> : null}
          </div>
        </CardHeader>
        <CardContent className={contentClassName}>{children}</CardContent>
      </Card>
    );
  }

  return (
    <Card className={cn("shadow-none", className)}>
      <Accordion
        type="single"
        collapsible
        defaultValue={defaultOpen ? "section" : undefined}
      >
        <AccordionItem value="section" className="border-0">
          <CardHeader className="py-0">
            <div className="flex items-center justify-between gap-2">
              <AccordionTrigger className="flex-1 py-4 hover:no-underline">
                <CardTitle className="flex items-center gap-2 text-start">
                  <span>{title}</span>
                  {badge != null ? (
                    <Badge
                      variant="secondary"
                      className="font-normal tabular-nums"
                    >
                      {badge}
                    </Badge>
                  ) : null}
                </CardTitle>
              </AccordionTrigger>
              {action ? (
                <div className="shrink-0 py-4">{action}</div>
              ) : null}
            </div>
          </CardHeader>
          <AccordionContent className="px-4 pb-6">
            <div className={cn(contentClassName)}>{children}</div>
          </AccordionContent>
        </AccordionItem>
      </Accordion>
    </Card>
  );
}
