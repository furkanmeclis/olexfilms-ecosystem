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

import type { StorageFileKind } from "@/features/storage/types";

export function fileKindIcon(kind: StorageFileKind): LucideIcon {
  switch (kind) {
    case "folder":
      return Folder;
    case "pdf":
      return FileText;
    case "image":
      return Image;
    case "video":
      return Video;
    case "audio":
      return Music;
    case "archive":
      return FileArchive;
    case "document":
      return FileText;
    case "spreadsheet":
      return FileSpreadsheet;
    case "code":
      return FileCode;
    default:
      return File;
  }
}

export function formatBytes(bytes: number) {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  );
  const value = bytes / 1024 ** i;
  return `${value >= 10 || i === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[i]}`;
}

export function formatSpeed(bps?: number) {
  if (!bps) return "";
  return `${formatBytes(bps)}/s`;
}

export function parentPrefix(key: string) {
  const trimmed = key.replace(/\/$/, "");
  const index = trimmed.lastIndexOf("/");
  if (index < 0) return "";
  return `${trimmed.slice(0, index + 1)}`;
}

export function joinKey(prefix: string, name: string) {
  const cleanPrefix = prefix.replace(/^\/+/, "");
  const cleanName = name.replace(/^\/+|\/+$/g, "");
  if (!cleanPrefix) return cleanName;
  return `${cleanPrefix.replace(/\/?$/, "/")}${cleanName}`;
}

export function breadcrumbSegments(prefix: string) {
  if (!prefix) return [];
  const parts = prefix.replace(/\/$/, "").split("/").filter(Boolean);
  return parts.map((part, index) => ({
    name: part,
    prefix: `${parts.slice(0, index + 1).join("/")}/`,
  }));
}
