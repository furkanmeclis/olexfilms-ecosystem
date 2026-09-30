"use client";

import { Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useCommandPalette } from "@/features/search-engine/providers/command-palette-provider";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export function SearchTrigger({
  className,
  compact = false,
}: {
  className?: string;
  compact?: boolean;
}) {
  const { setOpen } = useCommandPalette();
  const { t } = useLocale();

  if (compact) {
    return (
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={cn("size-9 shrink-0", className)}
        onClick={() => setOpen(true)}
        aria-label={t("search.trigger_label")}
      >
        <Search className="size-4" />
      </Button>
    );
  }

  return (
    <Button
      type="button"
      variant="outline"
      className={cn(
        "text-muted-foreground hidden h-9 w-full max-w-md justify-start gap-2 px-3 font-normal md:flex",
        className,
      )}
      onClick={() => setOpen(true)}
    >
      <Search className="size-4 shrink-0 opacity-60" />
      <span className="flex-1 truncate text-left text-sm">
        {t("search.trigger_label")}
      </span>
      <kbd className="bg-muted pointer-events-none hidden rounded border px-1.5 py-0.5 font-mono text-[10px] font-medium sm:inline-block">
        ⌘K
      </kbd>
    </Button>
  );
}
