"use client";

import { CheckCircle2, Loader2, Undo2, XCircle } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { Button } from "@/components/ui/button";
import { ImportErrorReportButton } from "@/features/io/components/import-error-report-button";
import { ImportStatusCards } from "@/features/io/components/import-status-cards";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { rollbackWindowOpen } from "@/features/io/lib/display";
import {
  importPlan,
  isImportJobRunning,
  isPartiallyUndone,
} from "@/features/io/lib/import-summary";
import { importsService } from "@/features/io/services/imports.service";
import type { ExportJobScope, ImportJob } from "@/features/io/types";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Poll interval while the job is queued / applying. */
const POLL_MS = 1500;

export type ImportResultLink = { href: string; label: string };

type ImportResultStepProps = {
  initialJob: ImportJob;
  scope: ExportJobScope;
  /** Import job detail page (tenant or platform route). */
  detailHref: string;
  jobsHref?: string;
  /** Resource-specific link to what the import created (e.g. stock entry). */
  resultLink?: ImportResultLink | null;
  /** Called once the job reached applied / failed / rolled back. */
  onSettled?: (job: ImportJob) => void;
};

/** Step 5: follows the job, then shows the outcome and the undo. */
export function ImportResultStep({
  initialJob,
  scope,
  detailHref,
  jobsHref,
  resultLink,
  onSettled,
}: ImportResultStepProps) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const [nowMs, setNowMs] = useState(() => Date.now());
  const jobQuery = useQuery({
    queryKey: ioKeys.imports.detail(initialJob.uuid, scope),
    queryFn: () => importsService.get(initialJob.uuid, scope),
    initialData: initialJob,
    refetchInterval: (q) =>
      isImportJobRunning(q.state.data?.status) ? POLL_MS : false,
  });
  const job = jobQuery.data ?? initialJob;
  const running = isImportJobRunning(job.status);

  const settledStatus = useRef<string | null>(null);
  useEffect(() => {
    if (running || settledStatus.current === job.status) return;
    settledStatus.current = job.status;
    setNowMs(Date.now());
    onSettled?.(job);
  }, [job, running, onSettled]);

  const rollback = useAppMutation({
    mutationFn: () => importsService.rollback(job.uuid, scope),
    onSuccess: (updated) => {
      queryClient.setQueryData(ioKeys.imports.detail(job.uuid, scope), updated);
      void queryClient.invalidateQueries({
        queryKey: ioKeys.imports.lists(scope),
      });
      appToast.success(t("imports.toast.rollback_success"));
    },
  });

  const summary = job.preview_summary;
  const plan = importPlan(summary);
  const applied = summary?.counts?.applied ?? summary?.valid ?? 0;
  const canRollback =
    job.status === "applied" && rollbackWindowOpen(job.rollback_until, nowMs);
  const partial = isPartiallyUndone(job);

  return (
    <div
      className="space-y-6"
      data-testid="import-result"
      data-status={job.status}
    >
      {running ? (
        <div className="flex items-center gap-3 rounded-lg border p-5">
          <Loader2 className="text-primary size-6 animate-spin" />
          <div>
            <p className="font-medium">{t("imports.result.running_title")}</p>
            <p className="text-muted-foreground text-sm">
              {t("imports.result.running_hint")}
            </p>
          </div>
        </div>
      ) : job.status === "failed" ? (
        <div className="border-destructive/40 bg-destructive/5 flex items-start gap-3 rounded-lg border p-5">
          <XCircle className="text-destructive size-6 shrink-0" />
          <div className="space-y-1">
            <p className="font-medium">{t("imports.result.failed_title")}</p>
            {job.error ? (
              <p className="text-destructive text-sm whitespace-pre-wrap">
                {job.error}
              </p>
            ) : null}
          </div>
        </div>
      ) : job.status === "rolled_back" ? (
        <div className="flex items-start gap-3 rounded-lg border p-5">
          <Undo2 className="text-muted-foreground size-6 shrink-0" />
          <div className="space-y-1">
            <p className="font-medium" data-testid="import-result-title">
              {partial
                ? t("imports.result.partially_undone_title")
                : t("imports.result.undone_title")}
            </p>
            {partial ? (
              <p className="text-muted-foreground text-sm">
                {t("imports.undo_rejected_hint")}
              </p>
            ) : null}
          </div>
        </div>
      ) : (
        <div className="flex items-start gap-3 rounded-lg border border-emerald-600/30 bg-emerald-600/5 p-5">
          <CheckCircle2 className="size-6 shrink-0 text-emerald-600" />
          <div className="space-y-1">
            <p className="font-medium" data-testid="import-result-title">
              {t("imports.result.applied_title")}
            </p>
            <p className="text-muted-foreground text-sm">
              {t("imports.result.applied_summary", {
                applied,
                skipped: Math.max(0, (summary?.total ?? 0) - applied),
              })}
            </p>
          </div>
        </div>
      )}

      {summary && !running ? <ImportStatusCards summary={summary} /> : null}

      <div className="flex flex-wrap items-center gap-2">
        {resultLink && job.status === "applied" ? (
          <Button asChild size="sm">
            <Link href={resultLink.href}>{resultLink.label}</Link>
          </Button>
        ) : null}
        <Button asChild variant="outline" size="sm">
          <Link href={detailHref} data-testid="import-result-detail">
            {t("imports.result.view_detail")}
          </Link>
        </Button>
        {jobsHref ? (
          <Button asChild variant="ghost" size="sm">
            <Link href={jobsHref}>{t("imports.view_jobs")}</Link>
          </Button>
        ) : null}
        {!running && plan.skipped > 0 ? (
          <ImportErrorReportButton jobUuid={job.uuid} scope={scope} />
        ) : null}
      </div>

      {canRollback ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4">
          <p className="text-muted-foreground text-sm">
            {job.rollback_until
              ? t("imports.result.undo_until", {
                  date: format.dateTime(job.rollback_until),
                })
              : t("imports.rollback_hint")}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={rollback.isPending}
            onClick={() => rollback.mutate()}
            data-testid="import-undo"
          >
            <Undo2 className="size-4" />
            {t("imports.rollback")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
