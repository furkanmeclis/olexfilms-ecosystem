"use client";

import { useQuery } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { exportsService } from "@/features/io/services/exports.service";
import type { ExportJobScope } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

type ExportJsonPreviewProps = {
  uuid: string;
  enabled?: boolean;
  scope?: ExportJobScope;
};

export function ExportJsonPreview({
  uuid,
  enabled = true,
  scope = "platform",
}: ExportJsonPreviewProps) {
  const { t } = useLocale();

  const previewQuery = useQuery({
    queryKey: [
      ...ioKeys.exports.detail(uuid, scope),
      "preview",
      "json",
    ] as const,
    queryFn: async () => {
      const { blob } = await exportsService.fetchFile(uuid, scope);
      const text = await blob.text();
      return JSON.stringify(JSON.parse(text), null, 2);
    },
    enabled,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });

  if (previewQuery.isLoading) {
    return <Loading label={t("exports.detail.preview_loading")} />;
  }

  if (previewQuery.isError) {
    return (
      <ErrorState
        title={t("exports.detail.preview_failed_title")}
        description={t("exports.detail.preview_failed_description")}
        onRetry={() => void previewQuery.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <ScrollArea className="h-[min(70vh,640px)] w-full rounded-md border">
      <pre className="p-4 text-xs leading-relaxed whitespace-pre-wrap">
        {previewQuery.data}
      </pre>
      <ScrollBar orientation="horizontal" />
    </ScrollArea>
  );
}

export function isJsonExportFormat(format: string) {
  return format.toLowerCase() === "json";
}
