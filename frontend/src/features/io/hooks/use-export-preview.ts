"use client";

import { useQuery } from "@tanstack/react-query";

import { ioKeys } from "@/features/io/hooks/query-keys";
import {
  isSpreadsheetExportFormat,
  parseExportBlob,
} from "@/features/io/lib/parse-export-spreadsheet";
import { exportsService } from "@/features/io/services/exports.service";
import type { ExportJobScope } from "@/features/io/types";

export function useExportPreview(
  uuid: string,
  format: string,
  enabled: boolean,
  scope: ExportJobScope = "platform",
) {
  const spreadsheetFormat = isSpreadsheetExportFormat(format) ? format : null;

  return useQuery({
    queryKey: [
      ...ioKeys.exports.detail(uuid, scope),
      "preview",
      format,
    ] as const,
    queryFn: async () => {
      if (!spreadsheetFormat) {
        throw new Error("Preview is not supported for this format.");
      }
      const { blob } = await exportsService.fetchFile(uuid, scope);
      return parseExportBlob(blob, spreadsheetFormat);
    },
    enabled: enabled && Boolean(spreadsheetFormat),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });
}
