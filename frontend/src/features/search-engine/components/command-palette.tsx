"use client";

import { Loader2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef } from "react";

import type { AppLayoutVariant } from "@/components/layout/app-layout";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import {
  paletteItemIcon,
  useCommandPaletteData,
} from "@/features/search-engine/hooks/use-command-palette-data";
import { pushRecentItem } from "@/features/search-engine/lib/recent";
import { useCommandPalette } from "@/features/search-engine/providers/command-palette-provider";
import type { PaletteItem } from "@/features/search-engine/types";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export function CommandPalette({
  variant,
  tenantSlug,
}: {
  variant: AppLayoutVariant;
  tenantSlug?: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { open, setOpen } = useCommandPalette();
  const data = useCommandPaletteData(variant, tenantSlug);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    const handle = requestAnimationFrame(() => inputRef.current?.focus());
    return () => cancelAnimationFrame(handle);
  }, [open]);

  const onSelect = useCallback(
    (item: PaletteItem) => {
      pushRecentItem(item);
      data.refreshRecent();
      setOpen(false);
      router.push(item.href);
    },
    [data, router, setOpen],
  );

  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      shouldFilter={false}
      inputRef={inputRef}
      title={t("search.title")}
      description={t("search.description")}
      className="max-w-2xl gap-0 overflow-hidden p-0 sm:max-w-2xl"
    >
      <div className="border-b px-3 py-2">
        <div className="flex flex-wrap gap-1.5">
          {data.specOptions.map((spec) => {
            const active = data.effectiveSpec === spec.id;
            const Icon = spec.icon;
            return (
              <button
                key={spec.id}
                type="button"
                onClick={() => data.setActiveSpec(active ? undefined : spec.id)}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors",
                  active
                    ? "bg-primary text-primary-foreground border-primary"
                    : "bg-muted/40 text-muted-foreground hover:bg-muted",
                )}
              >
                <Icon className="size-3.5" />
                {spec.label}
              </button>
            );
          })}
        </div>
      </div>

      <CommandInput
        ref={inputRef}
        placeholder={data.placeholder}
        value={data.query}
        onValueChange={data.setQuery}
      />

      <CommandList className="max-h-[min(420px,50vh)]">
        {data.loading ? (
          <div className="text-muted-foreground flex items-center justify-center gap-2 py-10 text-sm">
            <Loader2 className="size-4 animate-spin" />
            {t("common.loading")}
          </div>
        ) : (
          <>
            <CommandEmpty>{data.emptyText}</CommandEmpty>
            {[...data.groupedItems.entries()].map(([group, items], index) => (
              <div key={group}>
                {index > 0 ? <CommandSeparator /> : null}
                <CommandGroup heading={group}>
                  {items.map((item) => (
                    <CommandItem
                      key={item.id}
                      value={`${item.id} ${item.label} ${item.description ?? ""}`}
                      onSelect={() => onSelect(item)}
                      className="gap-3 py-3"
                    >
                      <span className="bg-muted flex size-8 shrink-0 items-center justify-center rounded-md border">
                        {paletteItemIcon(item)}
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="truncate font-medium">
                          {item.label}
                        </span>
                        {item.description ? (
                          <span className="text-muted-foreground truncate text-xs">
                            {item.description}
                          </span>
                        ) : null}
                      </span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </div>
            ))}
          </>
        )}
      </CommandList>

      <div className="text-muted-foreground flex items-center justify-between border-t px-4 py-2 text-xs">
        <span>{data.footerHint}</span>
        <div className="flex items-center gap-3">
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5 font-mono">↵</kbd>{" "}
            {t("search.footer_open")}
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5 font-mono">esc</kbd>{" "}
            {t("search.footer_close")}
          </span>
        </div>
      </div>
    </CommandDialog>
  );
}
