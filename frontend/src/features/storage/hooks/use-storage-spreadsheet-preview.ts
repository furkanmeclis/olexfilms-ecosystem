"use client";

import { useQuery } from "@tanstack/react-query";

import {
  parseExportBlob,
  resolveSpreadsheetPreviewFormat,
  type ParsedExportSheet,
} from "@/features/io/lib/parse-export-spreadsheet";
import { storageKeys } from "@/features/storage/hooks/query-keys";
import { platformDownloadFile } from "@/lib/api/platform-form-request";

async function fetchSpreadsheetFromUrl(
  url: string,
  format: "csv" | "xlsx",
): Promise<ParsedExportSheet> {
  const response = await fetch(url, { credentials: "omit" });
  if (!response.ok) {
    throw new Error(`Preview failed (${response.status})`);
  }
  const blob = await response.blob();
  return parseExportBlob(blob, format);
}

export function useStorageSpreadsheetPreview(
  key: string | null,
  filename: string,
  mimeType: string,
  enabled: boolean,
  versionId?: string,
) {
  const format = resolveSpreadsheetPreviewFormat(filename, mimeType);

  return useQuery({
    queryKey: [
      ...storageKeys.detail(key ?? ""),
      "spreadsheet-preview",
      format,
      versionId,
    ] as const,
    queryFn: async () => {
      if (!key || !format) {
        throw new Error("Spreadsheet preview is not supported for this file.");
      }
      const { blob } = await platformDownloadFile(
        "/v1/platform/storage/objects/preview",
        { key, version_id: versionId },
      );
      return parseExportBlob(blob, format);
    },
    enabled: enabled && Boolean(key && format),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });
}

export function usePublicSpreadsheetPreview(
  url: string | null,
  filename: string,
  mimeType: string,
  enabled: boolean,
) {
  const format = resolveSpreadsheetPreviewFormat(filename, mimeType);

  return useQuery({
    queryKey: [
      ...storageKeys.all,
      "public-spreadsheet-preview",
      url,
      format,
    ] as const,
    queryFn: async () => {
      if (!url || !format) {
        throw new Error("Spreadsheet preview is not supported for this file.");
      }
      return fetchSpreadsheetFromUrl(url, format);
    },
    enabled: enabled && Boolean(url && format),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });
}

export { resolveSpreadsheetPreviewFormat };
