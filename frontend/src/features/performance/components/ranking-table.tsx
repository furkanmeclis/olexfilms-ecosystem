"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Download, ListTodo, Target } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { useProvinces } from "@/features/geo/hooks/use-geo";
import {
  TargetDialog,
  TaskDialog,
} from "@/features/performance/components/ranking-dialogs";
import {
  formatMetric,
  PERFORMANCE_DEFAULT_COUNTRY,
  PERFORMANCE_METRICS,
  rankingCsv,
} from "@/features/performance/lib/performance";
import {
  performanceKeys,
  performanceService,
  type PerformanceRankingRow,
} from "@/features/performance/services/performance.service";
import { download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const RANKING_PERSIST_KEY = "tenant-performance-ranking-v1";
const EXPORT_PAGE = 100;

/**
 * Network ranking (center and distributor): GET /v1/performance/ranking
 * with every metric column sortable, name / distributor / province sort,
 * `q`, organization type (center), distributor (center) and province
 * facets, CSV download of every matching row and the row actions "open
 * task" (center with tasks.write) and "set target" (targets.manage; a
 * distributor only for its dealers).
 */
export function RankingTable({
  orgType,
  period,
}: {
  orgType: string;
  period: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const isCenter = orgType === "center";
  const canTask = isCenter && can(Permission.TasksWrite);
  const canTarget = can(Permission.PerformanceTargetsManage);
  const [targetRow, setTargetRow] = useState<PerformanceRankingRow | null>(
    null,
  );
  const [taskRow, setTaskRow] = useState<PerformanceRankingRow | null>(null);
  const [exporting, setExporting] = useState(false);

  const provinces = useProvinces(PERFORMANCE_DEFAULT_COUNTRY);
  const distributorQuery = {
    type: "distributor",
    sort: "name",
    limit: 100,
    offset: 0,
    period,
  };
  const distributors = useQuery({
    queryKey: performanceKeys.distributors(period),
    queryFn: () => performanceService.ranking(distributorQuery),
    enabled: isCenter,
    staleTime: 5 * 60 * 1000,
  });
  const provinceOptions = useMemo(
    () =>
      (provinces.data ?? []).map((p) => ({
        value: String(p.id),
        label: p.name,
      })),
    [provinces.data],
  );
  const distributorOptions = useMemo(
    () =>
      (distributors.data?.items ?? []).map((d) => ({
        value: d.organization_uuid ?? "",
        label: d.name ?? "",
      })),
    [distributors.data],
  );

  const columns = useMemo(() => {
    const cols: ColumnDef<PerformanceRankingRow, unknown>[] = [
      createColumn<PerformanceRankingRow>({
        accessorKey: "rank",
        labelKey: "performance.ranking.rank",
        enableSorting: false,
        enableColumnFilter: false,
        enableHiding: false,
        meta: { cellClassName: "text-end tabular-nums w-12" },
      }),
      createColumn<PerformanceRankingRow>({
        accessorKey: "name",
        labelKey: "performance.ranking.organization",
        enableSorting: true,
        enableHiding: false,
        enableColumnFilter: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div>
            <span className="font-medium">{row.original.name}</span>
            <div className="text-muted-foreground text-xs">
              {t(`performance.org_type.${row.original.type ?? "dealer"}`)}
            </div>
          </div>
        ),
      }),
      createColumn<PerformanceRankingRow>({
        accessorKey: "type",
        labelKey: "performance.ranking.type",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        enableColumnFilter: isCenter,
        filterOptions: ["distributor", "dealer"].map((v) => ({
          value: v,
          label: v,
          labelKey: `performance.org_type.${v}`,
        })),
        param: "type",
        cell: ({ row }) =>
          t(`performance.org_type.${row.original.type ?? "dealer"}`),
      }),
      createColumn<PerformanceRankingRow>({
        id: "distributor",
        accessorFn: (row) => row.distributor?.name ?? "",
        labelKey: "performance.ranking.distributor",
        enableSorting: true,
        gridSecondary: true,
        filterVariant: "faceted",
        filterOptions: distributorOptions,
        enableColumnFilter: isCenter && distributorOptions.length > 0,
        param: "distributor_uuid",
        cell: ({ row }) => row.original.distributor?.name ?? "—",
      }),
      createColumn<PerformanceRankingRow>({
        id: "province",
        accessorFn: (row) => row.province_name ?? "",
        labelKey: "performance.ranking.province",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: provinceOptions,
        enableColumnFilter: provinceOptions.length > 0,
        param: "province_id",
        cell: ({ row }) => row.original.province_name ?? "—",
      }),
      ...PERFORMANCE_METRICS.map((metric) =>
        createColumn<PerformanceRankingRow>({
          id: metric,
          accessorFn: (row) => row.metrics?.[metric]?.value ?? "",
          labelKey: `performance.metrics.${metric}`,
          enableSorting: true,
          enableColumnFilter: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            formatMetric(
              metric,
              row.original.metrics?.[metric],
              format,
              row.original.currency,
            ),
        }),
      ),
    ];
    if (canTask || canTarget) {
      cols.push(
        createColumn<PerformanceRankingRow>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => {
            const r = row.original;
            const actions: EntityRowAction[] = [];
            if (canTask) {
              actions.push({
                id: "task",
                label: t("performance.ranking.open_task"),
                icon: ListTodo,
                onSelect: () => setTaskRow(r),
              });
            }
            // A distributor sets targets only for its dealers.
            if (canTarget && (isCenter || r.type === "dealer")) {
              actions.push({
                id: "target",
                label: t("performance.ranking.set_target"),
                icon: Target,
                onSelect: () => setTargetRow(r),
              });
            }
            return <EntityRowActions actions={actions} />;
          },
        }),
      );
    }
    return cols;
  }, [
    canTarget,
    canTask,
    distributorOptions,
    format,
    isCenter,
    provinceOptions,
    t,
  ]);

  const listState = useServerListState({
    columns,
    initialSort: "-services_count",
    persistKey: RANKING_PERSIST_KEY,
  });
  const query = useMemo(
    () => ({ ...listState.params, period }),
    [listState.params, period],
  );
  const list = useQuery({
    queryKey: performanceKeys.ranking(query),
    queryFn: () => performanceService.ranking(query),
  });

  const exportCsv = async () => {
    setExporting(true);
    try {
      const rows: PerformanceRankingRow[] = [];
      for (let offset = 0; ; offset += EXPORT_PAGE) {
        const page = await performanceService.ranking({
          ...query,
          limit: EXPORT_PAGE,
          offset,
        });
        rows.push(...page.items);
        if (page.items.length === 0 || rows.length >= page.total) break;
      }
      download(
        new Blob([rankingCsv(rows)], { type: "text/csv;charset=utf-8" }),
        `performance-ranking-${period}.csv`,
      );
    } catch {
      appToast.error(t("exports.toast.failed"));
    } finally {
      setExporting(false);
    }
  };

  return (
    <div data-testid="performance-ranking">
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.organization_uuid ?? String(row.rank)}
        rowCount={list.data?.total ?? 0}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        state={listState.tableState}
        features={{
          persistKey: RANKING_PERSIST_KEY,
          rowSelection: false,
          columnOrdering: true,
          columnPinning: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          >
            <Button
              variant="outline"
              size="sm"
              disabled={exporting}
              onClick={() => void exportCsv()}
              data-testid="performance-ranking-export"
            >
              <Download className="size-4" />
              {t("performance.ranking.export")}
            </Button>
          </EntityToolbar>
        }
        emptyTitle={t("performance.empty")}
      />
      <TargetDialog
        row={targetRow}
        period={period}
        onOpenChange={(open) => {
          if (!open) setTargetRow(null);
        }}
      />
      <TaskDialog
        row={taskRow}
        onOpenChange={(open) => {
          if (!open) setTaskRow(null);
        }}
      />
    </div>
  );
}
