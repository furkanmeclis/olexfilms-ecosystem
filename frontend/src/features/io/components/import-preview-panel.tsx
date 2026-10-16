"use client";

import { useMemo, useState, type ReactNode } from "react";

import { ImportErrorReportButton } from "@/features/io/components/import-error-report-button";
import { ImportRowsTable } from "@/features/io/components/import-rows-table";
import { ImportStatusCards } from "@/features/io/components/import-status-cards";
import { importPlan, importTableRows } from "@/features/io/lib/import-summary";
import type {
  ExportJobScope,
  ImportPreviewSummary,
  ImportSchemaField,
} from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

type ImportPreviewPanelProps = {
  summary: ImportPreviewSummary;
  schema: ImportSchemaField[];
  jobUuid: string;
  scope?: ExportJobScope;
  /** Extra notes above the table (e.g. partial undo). */
  notice?: ReactNode;
};

/**
 * Status cards + row table of an import summary (wizard preview step and
 * import detail page). A card click filters the table by row status.
 */
export function ImportPreviewPanel({
  summary,
  schema,
  jobUuid,
  scope = "platform",
  notice,
}: ImportPreviewPanelProps) {
  const { t } = useLocale();
  const [status, setStatus] = useState<string | null>(null);
  const rows = useMemo(() => importTableRows(summary), [summary]);
  const visible = useMemo(
    () => (status ? rows.filter((row) => row.status === status) : rows),
    [rows, status],
  );
  const plan = importPlan(summary);
  const hasProblems = plan.skipped > 0 || (summary.errors?.length ?? 0) > 0;
  const truncated = rows.length < (summary.total ?? 0) && !status;

  return (
    <div className="space-y-4" data-testid="import-preview-panel">
      <ImportStatusCards
        summary={summary}
        selected={status}
        onSelect={setStatus}
      />
      {notice}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-xs">
          {status
            ? t("imports.table.filtered", { count: visible.length })
            : truncated
              ? t("imports.table.truncated", {
                  shown: rows.length,
                  total: summary.total ?? 0,
                })
              : t("imports.table.hint")}
        </p>
        {hasProblems ? (
          <ImportErrorReportButton jobUuid={jobUuid} scope={scope} />
        ) : null}
      </div>
      <ImportRowsTable rows={visible} schema={schema} />
    </div>
  );
}
