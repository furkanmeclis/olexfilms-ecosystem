"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import {
  CLIENT_SIDE_MANUAL,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { AchievementBar } from "@/features/performance/components/achievement-bar";
import { numberValue } from "@/features/performance/lib/performance";
import {
  monthColumnId,
  monthOfColumn,
  STAFF_METRICS,
  teamMonths,
  teamRows,
  type StaffMetric,
  type TeamRow,
} from "@/features/performance/lib/targets";
import {
  performanceKeys,
  performanceService,
} from "@/features/performance/services/performance.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const TEAM_PERSIST_KEY = "tenant-performance-team-targets-v1";

/**
 * Team targets (TEC-497, dealer, performance.staff_targets.manage): staff
 * × month grid of one metric around the picked month. Double-click a
 * month cell to set the target (empty clears it); the bar under the
 * target is the staff member's achievement of that month.
 */
export function TeamTargetsTab({ period }: { period: string }) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [metric, setMetric] = useState<StaffMetric>("services_count");
  const months = useMemo(() => teamMonths(period), [period]);
  const from = months[0];
  const to = months[months.length - 1];

  const members = useQuery({
    queryKey: performanceKeys.members,
    queryFn: () => performanceService.members(),
  });
  const targets = useQuery({
    queryKey: performanceKeys.staffTargets(from, to),
    queryFn: () =>
      performanceService.staffTargets({ period_from: from, period_to: to }),
  });
  const rows = useMemo(
    () => teamRows(members.data ?? [], targets.data ?? [], metric),
    [members.data, targets.data, metric],
  );

  const save = useMutation({
    mutationFn: async (input: {
      row: TeamRow;
      month: string;
      value: string;
    }) => {
      const current = input.row.months[input.month];
      if (!input.value) {
        if (current?.uuid) {
          await performanceService.deleteStaffTarget(current.uuid);
        }
        return;
      }
      await performanceService.upsertStaffTarget({
        user_uuid: input.row.uuid,
        period: input.month,
        metric,
        value: input.value,
      });
    },
    onSuccess: () => {
      appToast.success(t("performance.team.saved"));
      void qc.invalidateQueries({
        queryKey: performanceKeys.staffTargets(from, to),
      });
    },
    onError: () => appToast.error(t("performance.team.failed")),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<TeamRow>({
          accessorKey: "name",
          labelKey: "performance.team.staff",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        ...months.map((month) =>
          createColumn<TeamRow>({
            id: monthColumnId(month),
            accessorFn: (row) => row.months[month]?.value ?? "",
            labelKey: "performance.team.month",
            header: () =>
              format.dateParts(`${month}-01T12:00:00`, {
                month: "short",
                year: "numeric",
              }),
            enableSorting: true,
            editVariant: "number",
            meta: { cellClassName: month === period ? "bg-muted/40" : "" },
            cell: ({ row }) => {
              const target = row.original.months[month];
              if (!target) {
                return (
                  <span
                    className="text-muted-foreground text-xs"
                    data-testid={`team-cell-${row.original.uuid}-${month}`}
                  >
                    —
                  </span>
                );
              }
              const value = numberValue(target.value) ?? 0;
              return (
                <div
                  className="space-y-1"
                  data-testid={`team-cell-${row.original.uuid}-${month}`}
                >
                  <div className="text-end text-sm tabular-nums">
                    {target.currency
                      ? format.currency(value, target.currency)
                      : format.number(value)}
                  </div>
                  <AchievementBar pct={target.achievement_pct} />
                </div>
              );
            },
          }),
        ),
      ] as ColumnDef<TeamRow, unknown>[],
    [format, months, period],
  );

  return (
    <div className="space-y-4" data-testid="performance-team-targets">
      <div className="flex flex-wrap items-center gap-2">
        {STAFF_METRICS.map((m) => (
          <Button
            key={m}
            type="button"
            size="sm"
            variant={metric === m ? "default" : "outline"}
            aria-pressed={metric === m}
            data-testid={`team-metric-${m}`}
            onClick={() => setMetric(m)}
          >
            {t(`performance.team.metrics.${m}`)}
          </Button>
        ))}
        <p className="text-muted-foreground text-xs">
          {t("performance.team.hint")}
        </p>
      </div>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        isLoading={members.isLoading || targets.isLoading}
        isError={members.isError || targets.isError}
        onRetry={() => {
          void members.refetch();
          void targets.refetch();
        }}
        emptyTitle={t("performance.team.empty_title")}
        emptyDescription={t("performance.team.empty_description")}
        manual={CLIENT_SIDE_MANUAL}
        features={{
          persistKey: TEAM_PERSIST_KEY,
          rowSelection: false,
          inlineEdit: true,
          columnPinning: true,
        }}
        onCellEdit={({ row, columnId, value }) => {
          const month = monthOfColumn(columnId);
          if (!month) return;
          const raw = value == null ? "" : String(value).trim();
          if (raw && !((numberValue(raw) ?? 0) > 0)) {
            appToast.error(t("performance.team.invalid"));
            return;
          }
          save.mutate({ row, month, value: raw });
        }}
        renderGridItem={(row) => (
          <div className="space-y-2">
            <div className="font-medium">{row.name}</div>
            {row.months[period] ? (
              <AchievementBar pct={row.months[period]?.achievement_pct} />
            ) : (
              <p className="text-muted-foreground text-xs">
                {t("performance.team.no_target")}
              </p>
            )}
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void members.refetch();
              void targets.refetch();
            }}
            refreshDisabled={members.isFetching || targets.isFetching}
          />
        }
      />
    </div>
  );
}
