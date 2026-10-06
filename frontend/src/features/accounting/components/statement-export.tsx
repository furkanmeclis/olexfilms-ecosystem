"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Download, Loader2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  STATEMENT_EXPORT_FORMATS,
  type AccountingExportFormat,
  type AccountingExportJob,
  type StatementPeriod,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Poll interval of a queued / processing export job. */
export const EXPORT_POLL_MS = 2000;

const RUNNING: AccountingExportJob["status"][] = ["queued", "processing"];

/**
 * Accounting export job (TEC-175, TEC-379): `request` queues a job on
 * worker-docs, the job is polled until it completes and the file is then
 * downloaded once through /v1/accounting/exports/{uuid}/download.
 */
export function useAccountingExport({
  orgUuid,
  request,
  pollMs = EXPORT_POLL_MS,
}: {
  orgUuid: string;
  request: (format: AccountingExportFormat) => Promise<AccountingExportJob>;
  pollMs?: number;
}) {
  const { t } = useLocale();
  const [job, setJob] = useState<AccountingExportJob | null>(null);
  const downloaded = useRef<string | null>(null);

  const start = useMutation({
    mutationFn: request,
    onSuccess: (queued) => {
      downloaded.current = null;
      setJob(queued);
    },
    onError: (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("exports.toast.failed"),
      );
    },
  });

  const running = Boolean(job && RUNNING.includes(job.status));
  const poll = useQuery({
    queryKey: accountingKeys.exportJob(orgUuid, job?.uuid ?? ""),
    queryFn: () => accountingService.getExport(job!.uuid),
    enabled: running,
    refetchInterval: (q) => {
      const status = q.state.data?.status;
      return !status || RUNNING.includes(status) ? pollMs : false;
    },
  });

  const current = poll.data && poll.data.uuid === job?.uuid ? poll.data : job;

  useEffect(() => {
    if (!current) return;
    if (current.status === "completed" && downloaded.current !== current.uuid) {
      downloaded.current = current.uuid;
      accountingService
        .downloadExport(current)
        .then(() => appToast.success(t("accounting.statement.export_done")))
        .catch(() => appToast.error(t("exports.toast.failed")));
    }
    if (current.status === "failed" && downloaded.current !== current.uuid) {
      downloaded.current = current.uuid;
      appToast.error(current.error || t("exports.toast.failed"));
    }
  }, [current, t]);

  const busy =
    start.isPending || Boolean(current && RUNNING.includes(current.status));

  return { start, current, busy };
}

/** Statement export (TEC-175): a button per format, status and re-download. */
export function StatementExport({
  orgUuid,
  cariUuid,
  period,
  pollMs = EXPORT_POLL_MS,
}: {
  orgUuid: string;
  cariUuid: string;
  period: StatementPeriod;
  pollMs?: number;
}) {
  const { t, locale } = useLocale();
  const { start, current, busy } = useAccountingExport({
    orgUuid,
    pollMs,
    request: (format) =>
      accountingService.exportStatement(cariUuid, format, period, locale),
  });

  return (
    <div
      className="flex flex-wrap items-center gap-2"
      data-testid="statement-export"
      data-status={current?.status ?? "idle"}
    >
      {STATEMENT_EXPORT_FORMATS.map((format) => (
        <Button
          key={format}
          type="button"
          variant="outline"
          size="sm"
          disabled={busy}
          data-testid={`statement-export-${format}`}
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
        <span
          className="text-muted-foreground text-xs"
          role="status"
          data-testid="statement-export-status"
        >
          {t(`accounting.statement.export_status.${current.status}`)}
        </span>
      ) : null}
      {current?.status === "completed" ? (
        <Button
          type="button"
          variant="link"
          size="sm"
          data-testid="statement-export-download"
          onClick={() =>
            void accountingService
              .downloadExport(current)
              .catch(() => appToast.error(t("exports.toast.failed")))
          }
        >
          {t("accounting.statement.download_again")}
        </Button>
      ) : null}
    </div>
  );
}
