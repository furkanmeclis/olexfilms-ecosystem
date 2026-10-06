"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { DriftReportView } from "@/features/integrations/glorian/components/drift-report";
import { RunStatusBadge } from "@/features/integrations/glorian/components/glorian-status";
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
  type GlorianSyncRunFilter,
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

export const SYNC_RUNS_PERSIST_KEY = "platform-glorian-sync-runs-v1";

/**
 * Sync runs (server list: sort, CSV kind/status, started range, paging)
 * and the manual sync buttons. A reconcile run's drift report opens below
 * the table.
 */
export function GlorianSyncRuns({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState<string | null>(null);

  const columns = useMemo<ColumnDef<GlorianSyncRun, unknown>[]>(
    () => [
      createColumn<GlorianSyncRun>({
        accessorKey: "kind",
        labelKey: "integrations.glorian.runs.kind",
        enableSorting: true,
        filterVariant: "faceted",
        param: "kind",
        gridPrimary: true,
        filterOptions: RUN_KINDS.map((value) => ({
          value,
          labelKey: `integrations.glorian.run_kind.${value}`,
          label: value,
        })),
        cell: ({ row }) =>
          t(`integrations.glorian.run_kind.${row.original.kind}`),
      }),
      createColumn<GlorianSyncRun>({
        accessorKey: "status",
        labelKey: "integrations.glorian.runs.status",
        enableSorting: true,
        filterVariant: "faceted",
        param: "status",
        gridSecondary: true,
        filterOptions: RUN_STATUSES.map((value) => ({
          value,
          labelKey: `integrations.glorian.run_status.${value}`,
          label: value,
        })),
        cell: ({ row }) => <RunStatusBadge status={row.original.status} />,
      }),
      createColumn<GlorianSyncRun>({
        accessorKey: "started_at",
        labelKey: "integrations.glorian.runs.started",
        enableSorting: true,
        filterVariant: "date-range",
        param: "started",
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {format.dateTime(row.original.started_at)}
          </span>
        ),
      }),
      createColumn<GlorianSyncRun>({
        accessorKey: "finished_at",
        labelKey: "integrations.glorian.runs.finished",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {row.original.finished_at
              ? format.dateTime(row.original.finished_at)
              : "—"}
          </span>
        ),
      }),
      createColumn<GlorianSyncRun>({
        id: "counts",
        labelKey: "integrations.glorian.runs.counts",
        enableSorting: false,
        cell: ({ row }) => (
          <RunCounts
            run={row.original}
            open={open === row.original.uuid}
            onToggle={() =>
              setOpen((prev) =>
                prev === row.original.uuid ? null : row.original.uuid,
              )
            }
          />
        ),
      }),
      createColumn<GlorianSyncRun>({
        accessorKey: "error",
        labelKey: "integrations.glorian.runs.error",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-destructive break-all">
            {row.original.error ?? ""}
          </span>
        ),
      }),
    ],
    [format, open, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-started_at",
    initialPageSize: 20,
    persistKey: SYNC_RUNS_PERSIST_KEY,
  });
  const query = listState.params as GlorianSyncRunFilter;

  const runs = useQuery({
    queryKey: glorianKeys.runs(query),
    queryFn: () => glorianService.syncRunsPage(query),
    placeholderData: (previous) => previous,
    refetchInterval: (q) =>
      q.state.data?.items.some((r) => r.status === "running") ? 5_000 : false,
  });
  const openRun = runs.data?.items.find((run) => run.uuid === open);

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
      {canManage ? (
        <div className="flex flex-wrap justify-end gap-2">
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

      <div data-testid="sync-runs">
        <EntityTable
          columns={columns}
          data={runs.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={runs.isLoading}
          isError={runs.isError}
          errorTitle={glorianErrorText(t, runs.error)}
          onRetry={() => void runs.refetch()}
          emptyTitle={t("integrations.glorian.runs.empty")}
          emptyDescription=""
          rowCount={runs.data?.total ?? 0}
          state={listState.tableState}
          features={{
            persistKey: SYNC_RUNS_PERSIST_KEY,
            rowSelection: false,
          }}
          toolbarExtra={
            <EntityToolbar
              onRefresh={() => void runs.refetch()}
              refreshDisabled={runs.isFetching}
            />
          }
        />
      </div>

      {openRun ? (
        <DriftReportView report={parseDriftReport(openRun.counts)} />
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
