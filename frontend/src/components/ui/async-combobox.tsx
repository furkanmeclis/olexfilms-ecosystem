"use client";

import { Check, ChevronsUpDown, Loader2, X } from "lucide-react";
import { useEffect, useId, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";
import { foldSearch } from "@/lib/utils/search";

export type ComboboxOption = {
  value: string;
  label: string;
  description?: string;
  disabled?: boolean;
};

const EMPTY_OPTIONS: ComboboxOption[] = [];

export type AsyncComboboxProps = {
  value?: string;
  onValueChange?: (value: string) => void;
  /** Static options (client filter) */
  options?: ComboboxOption[];
  /**
   * Backend / remote search. Called with debounced query.
   * When provided, results replace static filtering.
   * Prefer a stable callback (module fn or useCallback).
   */
  loadOptions?: (query: string) => Promise<ComboboxOption[]>;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyText?: string;
  disabled?: boolean;
  className?: string;
  /** Debounce for loadOptions (ms). Default 300 */
  debounceMs?: number;
  /** Initial options shown before first search */
  initialOptions?: ComboboxOption[];
  /**
   * Extra values treated as selected in the open list (e.g. multi-picker chips).
   * Combined with `value` for checkmarks / "Selected" badge.
   */
  selectedValues?: readonly string[];
  /**
   * Hide options whose value is already in `selectedValues` / `value`.
   * Useful for multi-add pickers so chips and list stay in sync.
   */
  hideSelected?: boolean;
  /** Show an inline clear control when a value is selected. */
  clearable?: boolean;
  id?: string;
  "aria-invalid"?: boolean;
};

/**
 * Searchable combobox — static options or async `loadOptions` for API-backed lists.
 */
export function AsyncCombobox({
  value,
  onValueChange,
  options = EMPTY_OPTIONS,
  loadOptions,
  placeholder,
  searchPlaceholder,
  emptyText,
  disabled,
  className,
  debounceMs = 300,
  initialOptions,
  selectedValues,
  hideSelected = false,
  clearable = false,
  id,
  "aria-invalid": ariaInvalid,
}: AsyncComboboxProps) {
  const { t } = useLocale();
  const listId = useId();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [remote, setRemote] = useState<ComboboxOption[]>(
    initialOptions ?? EMPTY_OPTIONS,
  );
  const [loading, setLoading] = useState(false);
  const isAsync = Boolean(loadOptions);

  const selectedSet = useMemo(() => {
    const next = new Set<string>(selectedValues ?? []);
    if (value) next.add(value);
    return next;
  }, [selectedValues, value]);

  useEffect(() => {
    if (!loadOptions || !open) return;

    let cancelled = false;
    const handle = window.setTimeout(() => {
      setLoading(true);
      void loadOptions(query)
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
  }, [query, open, loadOptions, debounceMs]);

  const catalog = useMemo(
    () => mergeOptions(options, remote, initialOptions ?? EMPTY_OPTIONS),
    [options, remote, initialOptions],
  );

  const selected = value
    ? catalog.find((opt) => opt.value === value)
    : undefined;

  const items = useMemo(() => {
    const base = isAsync
      ? remote
      : options.filter((opt) => {
          if (!query) return true;
          const q = foldSearch(query);
          return (
            foldSearch(opt.label).includes(q) ||
            foldSearch(opt.value).includes(q) ||
            (opt.description ? foldSearch(opt.description).includes(q) : false)
          );
        });

    // Keep the current value visible even if the latest async page omitted it.
    let next = base;
    if (selected && !base.some((opt) => opt.value === selected.value)) {
      next = [selected, ...base];
    }

    if (hideSelected && selectedSet.size) {
      next = next.filter((opt) => !selectedSet.has(opt.value));
    }

    return next;
  }, [isAsync, remote, options, query, selected, hideSelected, selectedSet]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-controls={listId}
          aria-invalid={ariaInvalid}
          disabled={disabled}
          className={cn(
            "h-9 w-full justify-between font-normal",
            !selected && "text-muted-foreground",
            ariaInvalid && "border-destructive",
            className,
          )}
        >
          <span className="truncate">
            {selected?.label ?? placeholder ?? t("form.combobox_placeholder")}
          </span>
          <span className="ms-2 flex shrink-0 items-center gap-1">
            {clearable && value && !disabled ? (
              <span
                role="button"
                tabIndex={-1}
                className="hover:bg-accent rounded-sm p-0.5 opacity-60 hover:opacity-100"
                aria-label={t("form.clear")}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  onValueChange?.("");
                  setOpen(false);
                }}
                onPointerDown={(event) => {
                  // Keep the popover closed; don't steal focus to the trigger.
                  event.preventDefault();
                  event.stopPropagation();
                }}
              >
                <X className="size-3.5" />
              </span>
            ) : null}
            <ChevronsUpDown className="size-4 opacity-50" />
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[var(--radix-popover-trigger-width)] p-0"
        align="start"
      >
        <Command shouldFilter={false}>
          <CommandInput
            placeholder={searchPlaceholder ?? t("form.combobox_search")}
            value={query}
            onValueChange={setQuery}
          />
          <CommandList id={listId}>
            {loading ? (
              <div className="text-muted-foreground flex items-center justify-center gap-2 py-6 text-sm">
                <Loader2 className="size-4 animate-spin" />
                {t("common.loading")}
              </div>
            ) : (
              <>
                <CommandEmpty>
                  {emptyText ?? t("form.combobox_empty")}
                </CommandEmpty>
                <CommandGroup>
                  {items.map((opt) => {
                    const isSelected = selectedSet.has(opt.value);
                    return (
                      <CommandItem
                        key={opt.value}
                        value={opt.value}
                        disabled={opt.disabled}
                        data-checked={isSelected || undefined}
                        onSelect={() => {
                          onValueChange?.(opt.value === value ? "" : opt.value);
                          setOpen(false);
                        }}
                      >
                        <Check
                          className={cn(
                            "size-4 shrink-0",
                            isSelected
                              ? "text-primary opacity-100"
                              : "opacity-0",
                          )}
                          aria-hidden={!isSelected}
                        />
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="truncate">{opt.label}</span>
                          {opt.description ? (
                            <span className="text-muted-foreground truncate text-xs">
                              {opt.description}
                            </span>
                          ) : null}
                        </span>
                        {isSelected ? (
                          <span className="text-primary ms-auto shrink-0 text-xs font-medium">
                            {t("form.combobox_selected")}
                          </span>
                        ) : null}
                      </CommandItem>
                    );
                  })}
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function mergeOptions(...groups: ComboboxOption[][]): ComboboxOption[] {
  const byValue = new Map<string, ComboboxOption>();
  for (const group of groups) {
    for (const opt of group) {
      byValue.set(opt.value, opt);
    }
  }
  return [...byValue.values()];
}
