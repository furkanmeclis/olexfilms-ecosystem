"use client";

import dynamic from "next/dynamic";
import { useCallback, useEffect, useRef } from "react";
import type {
  ClipboardEvent as ReactClipboardEvent,
  KeyboardEvent,
} from "react";
import { useTheme } from "next-themes";
import { FileSpreadsheet } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import type { ParsedExportSheet } from "@/features/io/lib/parse-export-spreadsheet";
import { EXPORT_PREVIEW_MAX_ROWS } from "@/features/io/lib/parse-export-spreadsheet";
import { useLocale } from "@/providers/locale-provider";

import "@/features/io/components/export-spreadsheet-preview.css";

const Spreadsheet = dynamic(
  () => import("react-spreadsheet").then((mod) => mod.default),
  {
    ssr: false,
    loading: () => <Loading label="" />,
  },
);

function blockClipboardEvent(event: ReactClipboardEvent) {
  event.preventDefault();
  event.stopPropagation();
}

function isClipboardShortcut(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey)) return false;
  const key = event.key.toLowerCase();
  return key === "c" || key === "v" || key === "x" || key === "a";
}

export function StorageSpreadsheetPreviewPanel({
  sheet,
  isLoading,
  isError,
  onRetry,
}: {
  sheet?: ParsedExportSheet;
  isLoading: boolean;
  isError: boolean;
  onRetry?: () => void;
}) {
  const { t } = useLocale();
  const { resolvedTheme } = useTheme();
  const previewRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const blockNativeClipboard = (event: globalThis.ClipboardEvent) => {
      const root = previewRef.current;
      if (!root) return;
      const active = document.activeElement;
      if (!active || !root.contains(active)) return;
      event.preventDefault();
      event.stopImmediatePropagation();
    };

    document.addEventListener("copy", blockNativeClipboard, true);
    document.addEventListener("cut", blockNativeClipboard, true);
    document.addEventListener("paste", blockNativeClipboard, true);

    return () => {
      document.removeEventListener("copy", blockNativeClipboard, true);
      document.removeEventListener("cut", blockNativeClipboard, true);
      document.removeEventListener("paste", blockNativeClipboard, true);
    };
  }, []);

  const handlePreviewKeyDown = useCallback((event: KeyboardEvent) => {
    if (isClipboardShortcut(event)) {
      event.preventDefault();
      event.stopPropagation();
    }
  }, []);

  if (isLoading) {
    return <Loading label={t("exports.detail.preview_loading")} />;
  }

  if (isError) {
    return (
      <ErrorState
        title={t("exports.detail.preview_failed_title")}
        description={t("exports.detail.preview_failed_description")}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  }

  if (!sheet || sheet.data.length === 0) {
    return (
      <div className="text-muted-foreground flex items-center gap-2 text-sm">
        <FileSpreadsheet className="size-4 shrink-0" />
        {t("exports.detail.preview_empty")}
      </div>
    );
  }

  return (
    <div className="w-full space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="secondary">
          {t("exports.detail.preview_rows", {
            shown: sheet.data.length,
            total: sheet.totalRows,
          })}
        </Badge>
        <Badge variant="outline">
          {t("exports.detail.preview_columns", { count: sheet.columnCount })}
        </Badge>
        {sheet.truncated ? (
          <Badge variant="warning">
            {t("exports.detail.preview_truncated", {
              max: EXPORT_PREVIEW_MAX_ROWS,
            })}
          </Badge>
        ) : null}
      </div>

      <ScrollArea className="h-[min(60vh,640px)] w-full rounded-md border">
        <div
          ref={previewRef}
          className="min-w-max p-3 select-none"
          onCopy={blockClipboardEvent}
          onCut={blockClipboardEvent}
          onPaste={blockClipboardEvent}
          onCopyCapture={blockClipboardEvent}
          onCutCapture={blockClipboardEvent}
          onPasteCapture={blockClipboardEvent}
        >
          <Spreadsheet
            className="export-preview-spreadsheet"
            data={sheet.data}
            darkMode={resolvedTheme === "dark"}
            onKeyDown={handlePreviewKeyDown}
          />
        </div>
        <ScrollBar orientation="horizontal" />
      </ScrollArea>

      <p className="text-muted-foreground text-xs">
        {t("exports.detail.preview_readonly_hint")}
      </p>
    </div>
  );
}
