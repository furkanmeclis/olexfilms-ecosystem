"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { FileDown, FilePlus2, Loader2 } from "lucide-react";
import { useMemo, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { permissions } from "@/config/permissions";
import { lastClosedPeriods } from "@/features/fleets/lib/plan";
import {
  FLEET_REPORT_KINDS,
  FLEET_REPORT_STATUSES,
  fleetsService,
  type FleetCard,
  type FleetReport,
} from "@/features/fleets/services/fleets.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const FLEET_REPORTS_PERSIST_KEY = "tenant-fleet-reports-v1";

function reportTone(status: string) {
  if (status === "ready") return "success" as const;
  if (status === "failed") return "danger" as const;
  return "warning" as const;
}

function RequestDialog({
  fleetUuid,
  open,
  onOpenChange,
}: {
  fleetUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const defaults = useMemo(() => lastClosedPeriods(new Date()), []);
  const [kind, setKind] = useState<"monthly" | "quarterly">("monthly");
  const [period, setPeriod] = useState(defaults.month);

  const request = useMutation({
    mutationFn: () =>
      fleetsService.requestReport(fleetUuid, {
        period_kind: kind,
        period: period.trim(),
      }),
    onSuccess: async (report) => {
      appToast.success(
        t(
          report.status === "ready"
            ? "fleets.reports.already_ready"
            : "fleets.reports.requested",
        ),
      );
      await qc.invalidateQueries({
        queryKey: ["fleets", fleetUuid, "reports"],
      });
      onOpenChange(false);
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    request.mutate();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("fleets.reports.request")}</DialogTitle>
          <DialogDescription>
            {t("fleets.reports.request_hint")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <RadioGroup
            value={kind}
            onValueChange={(v) => {
              const next = v as "monthly" | "quarterly";
              setKind(next);
              setPeriod(next === "monthly" ? defaults.month : defaults.quarter);
            }}
            className="flex gap-4"
          >
            {FLEET_REPORT_KINDS.map((k) => (
              <div key={k} className="flex items-center gap-2">
                <RadioGroupItem value={k} id={`fleet-report-kind-${k}`} />
                <Label htmlFor={`fleet-report-kind-${k}`}>
                  {t(`fleets.reports.kind_${k}`)}
                </Label>
              </div>
            ))}
          </RadioGroup>
          <div className="space-y-1.5">
            <Label htmlFor="fleet-report-period">
              {t("fleets.reports.period")}
            </Label>
            <Input
              id="fleet-report-period"
              dir="ltr"
              value={period}
              placeholder={kind === "monthly" ? "2026-09" : "2026-Q3"}
              onChange={(e) => setPeriod(e.target.value)}
            />
          </div>
          {request.isError && isApiError(request.error) ? (
            <p className="text-destructive text-sm">{request.error.message}</p>
          ) : null}
          <DialogFooter>
            <Button type="submit" disabled={request.isPending}>
              {request.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              {t("fleets.reports.request")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Fleet card > Reports (TEC-477): the periodic reports of the fleet
 * (GET /v1/fleets/{uuid}/reports: status and kind facets, period sort),
 * the PDF of a ready one, and an on-demand report of a closed period.
 */
export function FleetReportsTab({ fleet }: { fleet: FleetCard }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const [requestOpen, setRequestOpen] = useState(false);

  const download = (report: FleetReport) =>
    void fleetsService
      .downloadReport(fleet.uuid, report)
      .catch(() => appToast.error(t("fleets.reports.download_failed")));

  const columns = useMemo(
    () =>
      [
        createColumn<FleetReport>({
          id: "period_start",
          accessorFn: (row) => row.period_start,
          labelKey: "fleets.reports.period",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) =>
            `${format.date(row.original.period_start)} – ${format.date(row.original.period_end)}`,
        }),
        createColumn<FleetReport>({
          accessorKey: "period_kind",
          labelKey: "fleets.reports.kind",
          enableSorting: false,
          filterVariant: "faceted",
          param: "period_kind",
          filterOptions: FLEET_REPORT_KINDS.map((value) => ({
            value,
            label: value,
            labelKey: `fleets.reports.kind_${value}`,
          })),
          cell: ({ row }) =>
            t(`fleets.reports.kind_${row.original.period_kind}`),
        }),
        createColumn<FleetReport>({
          accessorKey: "status",
          labelKey: "fleets.reports.status",
          enableSorting: false,
          filterVariant: "faceted",
          param: "status",
          filterOptions: FLEET_REPORT_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `fleets.reports.status_${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`fleets.reports.status_${row.original.status}`)}
              tone={reportTone(row.original.status)}
            />
          ),
        }),
        createColumn<FleetReport>({
          accessorKey: "locale",
          labelKey: "fleets.fields.report_locale",
          enableSorting: false,
          defaultHidden: true,
        }),
        createColumn<FleetReport>({
          accessorKey: "emailed_at",
          labelKey: "fleets.reports.emailed_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.emailed_at
              ? format.dateTime(row.original.emailed_at)
              : "—",
        }),
        createColumn<FleetReport>({
          accessorKey: "created_at",
          labelKey: "fleets.reports.created_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<FleetReport>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "download",
                  label: t("fleets.reports.download"),
                  icon: FileDown,
                  disabled: row.original.status !== "ready",
                  onSelect: () => download(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<FleetReport, unknown>[],
    // download closes over the fleet uuid and t.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [fleet.uuid, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-period_start",
    initialPageSize: 20,
    persistKey: FLEET_REPORTS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: ["fleets", fleet.uuid, "reports", params],
    queryFn: () => fleetsService.listReports(fleet.uuid, params),
  });

  return (
    <>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("fleets.reports.empty_title")}
        emptyDescription={t("fleets.reports.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: FLEET_REPORTS_PERSIST_KEY,
          globalFilter: false,
        }}
        toolbarExtra={
          <>
            {can(permissions.fleets.manage) ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setRequestOpen(true)}
              >
                <FilePlus2 className="size-4" />
                {t("fleets.reports.request")}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      <RequestDialog
        fleetUuid={fleet.uuid}
        open={requestOpen}
        onOpenChange={setRequestOpen}
      />
    </>
  );
}
