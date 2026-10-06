"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Star } from "lucide-react";
import { useMemo } from "react";

import type { FilterParamSpecs } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { StorageAccessBadge } from "@/features/storage/components/storage-access-badge";
import {
  StorageActionsMenu,
  StorageContextMenu,
  type StorageAction,
} from "@/features/storage/components/storage-context-menu";
import { StorageFileThumbnail } from "@/features/storage/components/storage-file-thumbnail";
import { formatBytes } from "@/features/storage/lib/format";
import type { StorageObject } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

export const STORAGE_FILES_PERSIST_KEY = "platform-storage-files-v1";

/**
 * `kind` filter values (backend `file_kind`, single value; folders always
 * stay visible).
 */
export const STORAGE_KIND_FILTER_VALUES = [
  "image",
  "video",
  "audio",
  "pdf",
  "document",
  "spreadsheet",
  "archive",
  "code",
] as const;

/** `access` filter values (single value). */
export const STORAGE_ACCESS_FILTER_VALUES = [
  "private",
  "public",
  "shared",
] as const;

/**
 * Column filter → list param map; same as the column meta, available
 * before the columns exist (the explorer's list state is created first).
 */
export const STORAGE_FILTER_PARAMS: FilterParamSpecs = {
  type: { param: "kind", filterVariant: "select" },
  access: { param: "access", filterVariant: "select" },
  updated_at: { param: "modified", filterVariant: "date-range" },
};

type StorageColumnsOptions = {
  canWrite: boolean;
  trash: boolean;
  onAction: (action: StorageAction, item: StorageObject) => void;
};

/**
 * Explorer list columns. Sort fields follow the backend whitelist
 * (name, size, updated_at, type); kind/access/modified map to the list
 * query params.
 */
export function useStorageColumns({
  canWrite,
  trash,
  onAction,
}: StorageColumnsOptions) {
  const { t, format } = useLocale();

  return useMemo<ColumnDef<StorageObject, unknown>[]>(
    () => [
      createSelectColumnDef<StorageObject>(),
      createColumn<StorageObject>({
        accessorKey: "name",
        labelKey: "storage.col_name",
        enableSorting: true,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <StorageContextMenu
            object={row.original}
            canWrite={canWrite}
            trash={trash}
            onAction={onAction}
          >
            <div className="flex max-w-md items-center gap-2">
              <StorageFileThumbnail
                object={row.original}
                className="size-9 shrink-0 rounded-md"
                iconClassName="size-4"
              />
              <span className="truncate font-medium">{row.original.name}</span>
              {row.original.is_starred ? (
                <Star className="size-3.5 shrink-0 fill-amber-400 text-amber-400" />
              ) : null}
            </div>
          </StorageContextMenu>
        ),
      }),
      createColumn<StorageObject>({
        // Column id = backend sort field `type`.
        id: "type",
        accessorKey: "file_kind",
        labelKey: "storage.col_type",
        enableSorting: true,
        filterVariant: "select",
        param: "kind",
        filterOptions: STORAGE_KIND_FILTER_VALUES.map((value) => ({
          value,
          labelKey: `storage.kind_${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {t(`storage.kind_${row.original.file_kind}`)}
          </span>
        ),
      }),
      createColumn<StorageObject>({
        accessorKey: "size",
        labelKey: "storage.col_size",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground tabular-nums">
            {row.original.kind === "folder"
              ? "—"
              : formatBytes(row.original.size)}
          </span>
        ),
      }),
      createColumn<StorageObject>({
        accessorKey: "updated_at",
        labelKey: "storage.col_modified",
        enableSorting: true,
        filterVariant: "date-range",
        param: "modified",
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {format.dateTime(row.original.updated_at)}
          </span>
        ),
      }),
      createColumn<StorageObject>({
        accessorKey: "access",
        labelKey: "storage.col_access",
        enableSorting: false,
        filterVariant: "select",
        param: "access",
        filterOptions: STORAGE_ACCESS_FILTER_VALUES.map((value) => ({
          value,
          labelKey: `storage.access_${value}`,
          label: value,
        })),
        cell: ({ row }) => <StorageAccessBadge access={row.original.access} />,
      }),
      createColumn<StorageObject>({
        id: "actions",
        labelKey: "storage.col_actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <StorageActionsMenu
            object={row.original}
            canWrite={canWrite}
            trash={trash}
            onAction={onAction}
          />
        ),
      }),
    ],
    [canWrite, format, onAction, t, trash],
  );
}

/** Grid / mobile card (DataTable `renderGridItem`). */
export function StorageFileCard({
  item,
  canWrite,
  trash,
  onAction,
}: {
  item: StorageObject;
} & StorageColumnsOptions) {
  const { t, format } = useLocale();

  return (
    <StorageContextMenu
      object={item}
      canWrite={canWrite}
      trash={trash}
      onAction={onAction}
    >
      <div className="flex flex-col items-center gap-2 text-center">
        <StorageFileThumbnail
          object={item}
          className="size-14 rounded-lg"
          iconClassName="size-7"
        />
        <div className="flex w-full items-center justify-center gap-1">
          <span className="truncate text-sm font-medium">{item.name}</span>
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
          {format.date(item.updated_at)}
        </span>
        <div className="flex items-center gap-1">
          <StorageAccessBadge access={item.access} />
          <StorageActionsMenu
            object={item}
            canWrite={canWrite}
            trash={trash}
            onAction={onAction}
          />
        </div>
      </div>
    </StorageContextMenu>
  );
}
