"use client";

import {
  File,
  FileArchive,
  FileCode,
  FileSpreadsheet,
  FileText,
  Folder,
  Image,
  Music,
  Video,
  type LucideIcon,
} from "lucide-react";

import { storageService } from "@/features/storage/services/storage.service";
import { supportsThumbnail } from "@/features/storage/lib/preview";
import type { StorageFileKind, StorageObject } from "@/features/storage/types";
import { cn } from "@/lib/utils";

const KIND_ICONS: Record<StorageFileKind, LucideIcon> = {
  folder: Folder,
  pdf: FileText,
  image: Image,
  video: Video,
  audio: Music,
  archive: FileArchive,
  document: FileText,
  spreadsheet: FileSpreadsheet,
  code: FileCode,
  unknown: File,
};

export function StorageFileThumbnail({
  object,
  className,
  iconClassName,
}: {
  object: StorageObject;
  className?: string;
  iconClassName?: string;
}) {
  if (supportsThumbnail(object)) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={storageService.previewUrl(object.key, object.version_id)}
        alt={object.name}
        loading="lazy"
        className={cn("bg-muted object-cover", className)}
      />
    );
  }

  const Icon = KIND_ICONS[object.file_kind] ?? File;

  return (
    <div
      className={cn(
        "bg-muted text-muted-foreground flex items-center justify-center",
        className,
      )}
    >
      <Icon className={cn("size-7", iconClassName)} />
    </div>
  );
}
