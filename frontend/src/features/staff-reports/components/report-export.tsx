"use client";

import { Download, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useAccountingExport } from "@/features/accounting/components/statement-export";
import {
  REPORT_EXPORT_FORMATS,
  staffReportsService,
  type ReportKind,
  type ReportQuery,
} from "@/features/staff-reports/services/staff-reports.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * CSV / XLSX / PDF of the open report: POST
 * /v1/accounting/reports/{kind}/export queues the job, which is polled and
 * downloaded like the statement export (TEC-175).
 */
export function ReportExport({
  orgUuid,
  kind,
  query,
  disabled,
  pollMs,
}: {
  orgUuid: string;
  kind: ReportKind;
  query: ReportQuery;
  disabled?: boolean;
  pollMs?: number;
}) {
  const { t, locale } = useLocale();
  const { start, current, busy } = useAccountingExport({
    orgUuid,
    pollMs,
    request: (format) =>
      staffReportsService.exportReport(kind, format, query, locale),
  });

  return (
    <div
      className="flex flex-wrap items-center gap-2"
      data-testid="report-export"
      data-status={current?.status ?? "idle"}
    >
      {REPORT_EXPORT_FORMATS.map((format) => (
        <Button
          key={format}
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || busy}
          data-testid={`report-export-${format}`}
          onClick={() => start.mutate(format)}
        >
          {busy &&
          (start.variables === format || current?.format === format) ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Download className="size-4" />
          )}
          {t(`exports.formats.${format}`)}
        </Button>
      ))}
      {current ? (
        <span className="text-muted-foreground text-xs" role="status">
          {t(`accounting.statement.export_status.${current.status}`)}
        </span>
      ) : null}
    </div>
  );
}
