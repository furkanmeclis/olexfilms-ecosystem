"use client";

import { ChevronRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { breadcrumbSegments } from "@/features/storage/lib/format";
import { useLocale } from "@/providers/locale-provider";

export function StorageBreadcrumb({
  prefix,
  onNavigate,
}: {
  prefix: string;
  onNavigate: (prefix: string) => void;
}) {
  const { t } = useLocale();
  const segments = breadcrumbSegments(prefix);

  return (
    <nav className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-7 px-2"
        onClick={() => onNavigate("")}
      >
        {t("storage.all_files")}
      </Button>
      {segments.map((segment) => (
        <span key={segment.prefix} className="flex items-center gap-1">
          <ChevronRight className="text-muted-foreground size-3.5 rtl:rotate-180" />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2"
            onClick={() => onNavigate(segment.prefix)}
          >
            {segment.name}
          </Button>
        </span>
      ))}
    </nav>
  );
}
