"use client";

import { Loader2 } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";

import type { ComboboxOption } from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type CommandBoxItem = ComboboxOption & {
  icon?: ReactNode;
  group?: string;
};

const EMPTY_ITEMS: CommandBoxItem[] = [];

type CommandBoxProps = {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  items?: CommandBoxItem[];
  /**
   * Prefer a stable callback (module fn or useCallback) to avoid refetch loops.
   */
  loadItems?: (query: string) => Promise<CommandBoxItem[]>;
  onSelect?: (item: CommandBoxItem) => void;
  title?: string;
  description?: string;
  placeholder?: string;
  emptyText?: string;
  debounceMs?: number;
  triggerLabel?: string;
  className?: string;
};

/**
 * Command palette style picker — static items or async `loadItems` (API search).
 */
export function CommandBox({
  open: openProp,
  onOpenChange,
  items = EMPTY_ITEMS,
  loadItems,
  onSelect,
  title,
  description,
  placeholder,
  emptyText,
  debounceMs = 300,
  triggerLabel,
  className,
}: CommandBoxProps) {
  const { t } = useLocale();
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const open = openProp ?? uncontrolledOpen;

  const [query, setQuery] = useState("");
  const [remote, setRemote] = useState<CommandBoxItem[]>(EMPTY_ITEMS);
  const [loading, setLoading] = useState(false);
  const isAsync = Boolean(loadItems);

  const setOpen = (next: boolean) => {
    if (!next) setQuery("");
    if (onOpenChange) onOpenChange(next);
    else setUncontrolledOpen(next);
  };

  useEffect(() => {
    if (!open || !loadItems) return;

    let cancelled = false;
    const handle = window.setTimeout(() => {
      setLoading(true);
      void loadItems(query)
        .then((next) => {
          if (!cancelled) setRemote(next);
        })
        .catch(() => {
          if (!cancelled) setRemote([]);
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, debounceMs);

    return () => {
      cancelled = true;
      window.clearTimeout(handle);
    };
  }, [open, query, loadItems, debounceMs]);

  const list = isAsync
    ? remote
    : items.filter((item) => {
        if (!query) return true;
        const q = query.toLowerCase();
        return (
          item.label.toLowerCase().includes(q) ||
          item.value.toLowerCase().includes(q) ||
          item.description?.toLowerCase().includes(q)
        );
      });

  const groups = list.reduce<Record<string, CommandBoxItem[]>>((acc, item) => {
    const key = item.group ?? t("form.command_group_default");
    acc[key] ??= [];
    acc[key].push(item);
    return acc;
  }, {});

  return (
    <>
      {triggerLabel ? (
        <Button
          type="button"
          variant="outline"
          className={cn(className)}
          onClick={() => setOpen(true)}
        >
          {triggerLabel}
        </Button>
      ) : null}

      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title={title ?? t("form.command_title")}
        description={description ?? t("form.command_description")}
      >
        <CommandInput
          placeholder={placeholder ?? t("form.combobox_search")}
          value={query}
          onValueChange={setQuery}
        />
        <CommandList>
          {loading ? (
            <div className="text-muted-foreground flex items-center justify-center gap-2 py-8 text-sm">
              <Loader2 className="size-4 animate-spin" />
              {t("common.loading")}
            </div>
          ) : (
            <>
              <CommandEmpty>
                {emptyText ?? t("form.combobox_empty")}
              </CommandEmpty>
              {Object.entries(groups).map(([group, groupItems], index) => (
                <div key={group}>
                  {index > 0 ? <CommandSeparator /> : null}
                  <CommandGroup heading={group}>
                    {groupItems.map((item) => (
                      <CommandItem
                        key={item.value}
                        value={item.value}
                        disabled={item.disabled}
                        onSelect={() => {
                          onSelect?.(item);
                          setOpen(false);
                        }}
                      >
                        {item.icon}
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="truncate">{item.label}</span>
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
      </CommandDialog>
    </>
  );
}
