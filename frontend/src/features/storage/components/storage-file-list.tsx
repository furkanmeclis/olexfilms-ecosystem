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
import { useLocale } from "@/providers/locale-provider";

export function StorageFileList({
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
  const { t, format } = useLocale();

  if (loading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-11 w-full" />
        ))}
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full min-w-[720px] text-sm">
        <thead className="bg-muted/40 text-muted-foreground border-b">
          <tr>
            <th className="w-10 px-3 py-2" />
            <th className="px-3 py-2 text-start font-medium">
              {t("storage.col_name")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("storage.col_type")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("storage.col_size")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("storage.col_modified")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("storage.col_access")}
            </th>
          </tr>
        </thead>
        <tbody>
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
                <tr
                  className={cn(
                    "hover:bg-muted/40 border-b last:border-b-0",
                    checked && "bg-accent/40",
                  )}
                  onDoubleClick={() => onOpen(item)}
                >
                  <td className="px-3 py-2">
                    <Checkbox
                      checked={checked}
                      onCheckedChange={(value) =>
                        onToggle(item.key, value === true)
                      }
                      aria-label={item.name}
                    />
                  </td>
                  <td className="px-3 py-2">
                    <button
                      type="button"
                      className="flex max-w-md items-center gap-2 text-start"
                      onClick={() => onOpen(item)}
                    >
                      <StorageFileThumbnail
                        object={item}
                        className="size-9 shrink-0 rounded-md"
                        iconClassName="size-4"
                      />
                      <span className="truncate font-medium">{item.name}</span>
                      {item.is_starred ? (
                        <Star className="size-3.5 fill-amber-400 text-amber-400" />
                      ) : null}
                    </button>
                  </td>
                  <td className="text-muted-foreground px-3 py-2">
                    {t(
                      `storage.kind_${item.file_kind}` as "storage.kind_unknown",
                    )}
                  </td>
                  <td className="text-muted-foreground px-3 py-2">
                    {item.kind === "folder" ? "—" : formatBytes(item.size)}
                  </td>
                  <td className="text-muted-foreground px-3 py-2">
                    {format.dateTime(item.updated_at)}
                  </td>
                  <td className="px-3 py-2">
                    <StorageAccessBadge access={item.access} />
                  </td>
                </tr>
              </StorageContextMenu>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
