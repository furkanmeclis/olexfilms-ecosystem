import type { StorageFileKind, StorageObject } from "@/features/storage/types";

export type PreviewKind =
  "image" | "video" | "audio" | "pdf" | "text" | "spreadsheet" | "none";

export function previewKindFromMime(mime: string): PreviewKind {
  const value = mime.toLowerCase();
  if (value.startsWith("image/")) return "image";
  if (value.startsWith("video/")) return "video";
  if (value.startsWith("audio/")) return "audio";
  if (value === "application/pdf") return "pdf";
  if (
    value.includes("spreadsheet") ||
    value.includes("excel") ||
    value === "text/csv" ||
    value === "text/tab-separated-values"
  ) {
    return "spreadsheet";
  }
  if (value.startsWith("text/") || value === "application/json") return "text";
  return "none";
}

export function previewKind(
  object: Pick<StorageObject, "file_kind" | "mime_type">,
): PreviewKind {
  if (object.file_kind === "image") return "image";
  if (object.file_kind === "video") return "video";
  if (object.file_kind === "audio") return "audio";
  if (object.file_kind === "pdf") return "pdf";
  if (object.file_kind === "spreadsheet") return "spreadsheet";
  if (object.file_kind === "code") return "text";
  return previewKindFromMime(object.mime_type);
}

export function isPreviewable(object: StorageObject) {
  if (object.kind === "folder") return false;
  return previewKind(object) !== "none";
}

export function supportsThumbnail(
  object: Pick<StorageObject, "kind" | "file_kind">,
) {
  return object.kind === "file" && object.file_kind === "image";
}

export function fileKindFromMime(mime: string): StorageFileKind {
  const value = mime.toLowerCase();
  if (value.startsWith("image/")) return "image";
  if (value.startsWith("video/")) return "video";
  if (value.startsWith("audio/")) return "audio";
  if (value === "application/pdf") return "pdf";
  if (value.includes("spreadsheet") || value.includes("excel"))
    return "spreadsheet";
  if (value.includes("zip") || value.includes("archive")) return "archive";
  if (value.startsWith("text/") || value === "application/json") return "code";
  if (value.includes("word") || value.includes("document")) return "document";
  return "unknown";
}

export function parseContentDispositionFilename(value: string | null) {
  if (!value) return null;
  const utf8 = /filename\*=UTF-8''([^;]+)/i.exec(value);
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1].trim());
    } catch {
      return utf8[1].trim();
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(value);
  return plain?.[1]?.trim() ?? null;
}
