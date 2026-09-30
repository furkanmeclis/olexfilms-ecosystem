"use client";

import { Star } from "lucide-react";

import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { StorageAccessBadge } from "@/features/storage/components/storage-access-badge";
import {
  StorageContextMenu,
  type StorageAction,
} from "@/features/storage/components/storage-context-menu";
import { StorageFileThumbnail } from "@/features/storage/components/storage-file-thumbnail";
import { formatBytes } from "@/features/storage/lib/format";
import type { StorageObject } from "@/features/storage/types";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function StorageFileGrid({
  items,
  loading,
  selected,
  canWrite,
  trash,
  onToggle,
  onOpen,
  onAction,
}: {
  items: StorageObject[];
  loading?: boolean;
  selected: Set<string>;
  canWrite: boolean;
  trash?: boolean;
  onToggle: (key: string, checked: boolean) => void;
  onOpen: (item: StorageObject) => void;
  onAction: (action: StorageAction, item: StorageObject) => void;
}) {
  const { t, locale } = useLocale();

  if (loading) {
    return (
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-4">
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-36 rounded-lg" />
        ))}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-4">
      {items.map((item) => {
        const checked = selected.has(item.key);
        return (
          <StorageContextMenu
            key={item.key}
            object={item}
            canWrite={canWrite}
            trash={trash}
            onAction={onAction}
          >
            <div
              className={cn(
                "bg-card relative rounded-lg border p-3 transition-colors",
                checked && "border-primary bg-accent/30",
              )}
              onDoubleClick={() => onOpen(item)}
            >
              <div className="absolute start-2 top-2 z-10">
                <Checkbox
                  checked={checked}
                  onCheckedChange={(value) =>
                    onToggle(item.key, value === true)
                  }
                  aria-label={item.name}
                />
              </div>
              <button
                type="button"
                className="flex w-full flex-col items-center gap-2 pt-6 text-center"
                onClick={() => onOpen(item)}
              >
                <StorageFileThumbnail
                  object={item}
                  className="size-14 rounded-lg"
                  iconClassName="size-7"
                />
                <div className="flex w-full items-center justify-center gap-1">
                  <span className="truncate text-sm font-medium">
                    {item.name}
                  </span>
                  {item.is_starred ? (
                    <Star className="size-3.5 shrink-0 fill-amber-400 text-amber-400" />
                  ) : null}
                </div>
                <span className="text-muted-foreground text-xs">
                  {item.kind === "folder"
                    ? t("storage.kind_folder")
                    : formatBytes(item.size)}
                </span>
                <span className="text-muted-foreground text-xs">
                  {datetime(item.updated_at, "dd.MM.yyyy", locale)}
                </span>
              </button>
              <div className="mt-2 flex justify-center">
                <StorageAccessBadge access={item.access} />
              </div>
            </div>
          </StorageContextMenu>
        );
      })}
    </div>
  );
}
