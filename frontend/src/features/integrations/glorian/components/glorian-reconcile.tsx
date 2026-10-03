"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Scale } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { DriftReportView } from "@/features/integrations/glorian/components/drift-report";
import { RunStatusBadge } from "@/features/integrations/glorian/components/glorian-status";
import { parseDriftReport } from "@/features/integrations/glorian/lib/drift";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import { glorianService } from "@/features/integrations/glorian/services/glorian.service";
import { useFormatter, useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Reconcile trigger and the drift report of the latest reconcile run
 * (polled while it runs). The report is read only on the backend.
 */
export function GlorianReconcile({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();

  const latest = useQuery({
    queryKey: glorianKeys.reconcile(),
    queryFn: async () => {
      const items = await glorianService.listSyncRuns({
        kind: "reconcile",
        limit: 1,
      });
      return items[0] ?? null;
    },
    refetchInterval: (q) =>
      q.state.data?.status === "running" ? 5_000 : false,
  });

  const start = useMutation({
    mutationFn: () => glorianService.reconcile(),
    onSuccess: async (run) => {
      appToast.success(t("integrations.glorian.reconcile.started"));
      queryClient.setQueryData(glorianKeys.reconcile(), run);
      await queryClient.invalidateQueries({
        queryKey: [...glorianKeys.all, "sync-runs"],
      });
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });

  const run = latest.data;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        {canManage ? (
          <Button
            type="button"
            onClick={() => start.mutate()}
            disabled={start.isPending || run?.status === "running"}
            data-testid="reconcile-start"
          >
            <Scale className="size-4" aria-hidden />
            {t("integrations.glorian.reconcile.start")}
          </Button>
        ) : null}
        {run ? (
          <div className="text-muted-foreground flex items-center gap-2 text-sm">
            <RunStatusBadge status={run.status} />
            <span>
              {t("integrations.glorian.reconcile.last_run", {
                at: format.dateTime(run.started_at),
              })}
            </span>
          </div>
        ) : null}
      </div>

      {latest.isLoading ? <Loading label={t("common.loading")} /> : null}
      {latest.isError ? (
        <ErrorState
          title={glorianErrorText(t, latest.error)}
          retryLabel={t("common.retry")}
          onRetry={() => latest.refetch()}
        />
      ) : null}
      {latest.isSuccess && !run ? (
        <p className="text-muted-foreground text-sm">
          {t("integrations.glorian.reconcile.never")}
        </p>
      ) : null}
      {run?.status === "running" ? (
        <p className="text-muted-foreground text-sm">
          {t("integrations.glorian.reconcile.running")}
        </p>
      ) : null}
      {run?.status === "failed" && run.error ? (
        <p className="text-destructive text-sm">{run.error}</p>
      ) : null}
      {run?.status === "succeeded" ? (
        <DriftReportView report={parseDriftReport(run.counts)} />
      ) : null}
    </div>
  );
}
