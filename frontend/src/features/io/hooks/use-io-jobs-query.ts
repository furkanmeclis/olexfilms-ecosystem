"use client";

import { useQuery } from "@tanstack/react-query";

import { ioKeys } from "@/features/io/hooks/query-keys";
import { exportsService } from "@/features/io/services/exports.service";
import { importsService } from "@/features/io/services/imports.service";
import type { ExportJobScope } from "@/features/io/types";

export function useExportJob(
  uuid: string,
  enabled = true,
  scope: ExportJobScope = "platform",
) {
  return useQuery({
    queryKey: ioKeys.exports.detail(uuid, scope),
    queryFn: () => exportsService.get(uuid, scope),
    enabled: Boolean(uuid) && enabled,
  });
}

export function useImportJob(
  uuid: string,
  enabled = true,
  scope: ExportJobScope = "platform",
) {
  return useQuery({
    queryKey: ioKeys.imports.detail(uuid, scope),
    queryFn: () => importsService.get(uuid, scope),
    enabled: Boolean(uuid) && enabled,
  });
}
