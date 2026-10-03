"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { Fragment, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { DriftReportView } from "@/features/integrations/glorian/components/drift-report";
import {
  RunStatusBadge,
  selectClass,
} from "@/features/integrations/glorian/components/glorian-status";
import {
  countEntries,
  parseDriftReport,
} from "@/features/integrations/glorian/lib/drift";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import {
  glorianService,
  type GlorianSyncKind,
  type GlorianSyncRun,
  type GlorianSyncRunKind,
  type GlorianSyncRunStatus,
} from "@/features/integrations/glorian/services/glorian.service";
import { useFormatter, useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const RUN_KINDS: GlorianSyncRunKind[] = [
  "pull_categories",
  "pull_products",
  "pull_dealers",
  "pull_stock",
  "push_barcodes",
  "outbound",
  "reconcile",
];
export const RUN_STATUSES: GlorianSyncRunStatus[] = [
  "running",
  "succeeded",
  "failed",
];
const SYNC_KINDS: GlorianSyncKind[] = [
  "pull",
  "push_barcodes",
  "outbound_replay",
];

/** Sync run table with kind/status filters and the manual sync buttons. */
export function GlorianSyncRuns({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<GlorianSyncRunKind | "">("");
  const [status, setStatus] = useState<GlorianSyncRunStatus | "">("");
  const [open, setOpen] = useState<string | null>(null);

  const runs = useQuery({
    queryKey: glorianKeys.runs(kind, status),
    queryFn: () =>
      glorianService.listSyncRuns({
        ...(kind ? { kind } : {}),
        ...(status ? { status } : {}),
      }),
    refetchInterval: (q) =>
      q.state.data?.some((r) => r.status === "running") ? 5_000 : false,
  });

  const trigger = useMutation({
    mutationFn: (k: GlorianSyncKind) => glorianService.triggerSync(k),
    onSuccess: async (res) => {
      appToast.success(
        t("integrations.glorian.sync.queued", {
          kind: t(`integrations.glorian.sync.kind.${res.kind}`),
        }),
      );
      await queryClient.invalidateQueries({
        queryKey: [...glorianKeys.all, "sync-runs"],
      });
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <label className="grid gap-1 text-sm">
          <span>{t("integrations.glorian.runs.filter_kind")}</span>
          <select
            className={selectClass}
            value={kind}
            onChange={(e) => setKind(e.target.value as GlorianSyncRunKind)}
            data-testid="runs-kind"
          >
            <option value="">{t("integrations.glorian.runs.all")}</option>
            {RUN_KINDS.map((k) => (
              <option key={k} value={k}>
                {t(`integrations.glorian.run_kind.${k}`)}
              </option>
            ))}
          </select>
        </label>
        <label className="grid gap-1 text-sm">
          <span>{t("integrations.glorian.runs.filter_status")}</span>
          <select
            className={selectClass}
            value={status}
            onChange={(e) => setStatus(e.target.value as GlorianSyncRunStatus)}
            data-testid="runs-status"
          >
            <option value="">{t("integrations.glorian.runs.all")}</option>
            {RUN_STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`integrations.glorian.run_status.${s}`)}
              </option>
            ))}
          </select>
        </label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => runs.refetch()}
          disabled={runs.isFetching}
        >
          <RefreshCw className="size-4" aria-hidden />
          {t("integrations.glorian.refresh")}
        </Button>
        {canManage ? (
          <div className="ms-auto flex flex-wrap gap-2">
            {SYNC_KINDS.map((k) => (
              <Button
                key={k}
                type="button"
                size="sm"
                variant={k === "pull" ? "default" : "outline"}
                onClick={() => trigger.mutate(k)}
                disabled={trigger.isPending}
                data-testid={`sync-${k}`}
              >
                {t(`integrations.glorian.sync.kind.${k}`)}
              </Button>
            ))}
          </div>
        ) : null}
      </div>

      {runs.isLoading ? <Loading label={t("common.loading")} /> : null}
      {runs.isError ? (
        <ErrorState
          title={glorianErrorText(t, runs.error)}
          retryLabel={t("common.retry")}
          onRetry={() => runs.refetch()}
        />
      ) : null}
      {runs.data && runs.data.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("integrations.glorian.runs.empty")}
        </p>
      ) : null}
      {runs.data && runs.data.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm" data-testid="sync-runs">
            <thead className="text-muted-foreground text-start text-xs">
              <tr className="border-b">
                <th className="p-2 text-start">
                  {t("integrations.glorian.runs.kind")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.runs.status")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.runs.started")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.runs.finished")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.runs.counts")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.runs.error")}
                </th>
              </tr>
            </thead>
            <tbody>
              {runs.data.map((run) => (
                <Fragment key={run.uuid}>
                  <tr className="border-b align-top">
                    <td className="p-2">
                      {t(`integrations.glorian.run_kind.${run.kind}`)}
                    </td>
                    <td className="p-2">
                      <RunStatusBadge status={run.status} />
                    </td>
                    <td className="p-2 whitespace-nowrap">
                      {format.dateTime(run.started_at)}
                    </td>
                    <td className="p-2 whitespace-nowrap">
                      {run.finished_at ? format.dateTime(run.finished_at) : "—"}
                    </td>
                    <td className="p-2">
                      <RunCounts
                        run={run}
                        open={open === run.uuid}
                        onToggle={() =>
                          setOpen(open === run.uuid ? null : run.uuid)
                        }
                      />
                    </td>
                    <td className="text-destructive p-2 break-all">
                      {run.error ?? ""}
                    </td>
                  </tr>
                  {open === run.uuid ? (
                    <tr className="border-b">
                      <td colSpan={6} className="p-2">
                        <DriftReportView
                          report={parseDriftReport(run.counts)}
                        />
                      </td>
                    </tr>
                  ) : null}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}

function RunCounts({
  run,
  open,
  onToggle,
}: {
  run: GlorianSyncRun;
  open: boolean;
  onToggle: () => void;
}) {
  const { t } = useLocale();
  if (run.kind === "reconcile") {
    if (run.status === "running") return <span>—</span>;
    const report = parseDriftReport(run.counts);
    return (
      <Button
        type="button"
        variant="link"
        size="sm"
        aria-expanded={open}
        onClick={onToggle}
      >
        {t("integrations.glorian.runs.drift_toggle", { total: report.total })}
      </Button>
    );
  }
  const entries = countEntries(run.counts);
  if (entries.length === 0) return <span>—</span>;
  return (
    <span className="font-mono text-xs">
      {entries.map(([k, v]) => `${k}: ${v}`).join(" · ")}
    </span>
  );
}
