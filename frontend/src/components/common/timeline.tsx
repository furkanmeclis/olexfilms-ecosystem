import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

export type TimelineItem = {
  id: string;
  title: string;
  description?: string;
  meta?: ReactNode;
};

export function Timeline({
  items,
  className,
}: {
  items: TimelineItem[];
  className?: string;
}) {
  return (
    <ol
      className={cn(
        "border-border relative ms-2 space-y-4 border-s ps-6",
        className,
      )}
    >
      {items.map((item) => (
        <li key={item.id} className="relative">
          <span className="bg-primary absolute -start-[1.9rem] top-1.5 h-2.5 w-2.5 rounded-full" />
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-sm font-medium">{item.title}</p>
              {item.description ? (
                <p className="text-muted-foreground text-sm">
                  {item.description}
                </p>
              ) : null}
            </div>
            {item.meta ? (
              <div className="text-muted-foreground text-xs">{item.meta}</div>
            ) : null}
          </div>
        </li>
      ))}
    </ol>
  );
}
