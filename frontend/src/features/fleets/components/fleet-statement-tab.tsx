"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Download, Loader2 } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import {
  FLEET_STATEMENT_EXPORT_FORMATS,
  FLEET_STATEMENT_KINDS,
  fleetKeys,
  fleetsService,
  type ExportJob,
  type FleetCard,
  type FleetStatementExportFormat,
  type FleetStatementLine,
} from "@/features/fleets/services/fleets.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const FLEET_STATEMENT_PERSIST_KEY = "tenant-fleet-statement-v1";
const EXPORT_POLL_MS = 2000;
const RUNNING = ["queued", "processing"];

function isoDay(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** Default period: the current month up to today. */
export function defaultStatementPeriod(today = new Date()) {
  return {
    from: isoDay(new Date(today.getFullYear(), today.getMonth(), 1)),
    to: isoDay(today),
  };
}

function Total({ label, value }: { label: string; value: ReactNode }) {
  return (
    <Card>
      <CardContent className="space-y-1 pt-6">
        <div className="text-muted-foreground text-xs">{label}</div>
        <div className="text-lg font-semibold tabular-nums" dir="ltr">
          {value}
        </div>
      </CardContent>
    </Card>
  );
}

/** Statement export job: queued, polled, downloaded once. */
function useStatementExport(fleetUuid: string) {
  const { t, locale } = useLocale();
  const [job, setJob] = useState<ExportJob | null>(null);
  const downloaded = useRef<string | null>(null);

  const start = useMutation({
    mutationFn: (v: {
      format: FleetStatementExportFormat;
      from: string;
      to: string;
    }) =>
      fleetsService.exportStatement(fleetUuid, {
        format: v.format,
        period_from: v.from,
        period_to: v.to,
        locale,
      }),
    onSuccess: (queued) => {
      downloaded.current = null;
      setJob(queued);
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("exports.toast.failed"),
      ),
  });

  const running = Boolean(job && RUNNING.includes(job.status));
  const poll = useQuery({
    queryKey: fleetKeys.exportJob(job?.uuid ?? ""),
    queryFn: () => fleetsService.getExport(job!.uuid),
    enabled: running,
    refetchInterval: (q) => {
      const status = q.state.data?.status;
      return !status || RUNNING.includes(status) ? EXPORT_POLL_MS : false;
    },
  });
  const current = poll.data && poll.data.uuid === job?.uuid ? poll.data : job;

  useEffect(() => {
    if (!current || downloaded.current === current.uuid) return;
    if (current.status === "completed") {
      downloaded.current = current.uuid;
      fleetsService
        .downloadExport(current)
        .catch(() => appToast.error(t("exports.toast.failed")));
    } else if (current.status === "failed") {
      downloaded.current = current.uuid;
      appToast.error(current.error || t("exports.toast.failed"));
    }
  }, [current, t]);

  const busy =
    start.isPending || Boolean(current && RUNNING.includes(current.status));
  return { start, current, busy };
}

/**
 * Fleet card > Statement (TEC-477, bulk invoice view): the fleet cari in
 * the caller's ledger over a period — service income lines with their
 * service and vehicle, collections, opening / closing balance and totals;
 * PDF / XLSX through the export engine.
 */
export function FleetStatementTab({ fleet }: { fleet: FleetCard }) {
  const { t, format } = useLocale();
  const params = useParams<{ slug: string }>();
  const slug = String(params?.slug ?? "");
  const initial = useMemo(() => defaultStatementPeriod(), []);
  const [from, setFrom] = useState(initial.from);
  const [to, setTo] = useState(initial.to);
  const valid = Boolean(from && to && from <= to);

  const statement = useQuery({
    queryKey: fleetKeys.statement(fleet.uuid, from, to),
    queryFn: () => fleetsService.statement(fleet.uuid, from, to),
    enabled: valid,
    retry: false,
  });
  const exporter = useStatementExport(fleet.uuid);
  const st = statement.data;
  const money = (v: string | undefined) =>
    v === undefined || v === ""
      ? "—"
      : format.currency(Number(v), st?.currency ?? "");

  const columns = useMemo(
    () =>
      [
        createColumn<FleetStatementLine>({
          accessorKey: "date",
          labelKey: "fleets.statement.date",
          enableSorting: true,
          cell: ({ row }) => format.date(row.original.date),
        }),
        createColumn<FleetStatementLine>({
          accessorKey: "kind",
          labelKey: "fleets.statement.kind",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: FLEET_STATEMENT_KINDS.map((value) => ({
            value,
            label: value,
            labelKey: `fleets.statement.kinds.${value}`,
          })),
          cell: ({ row }) => (
            <span>
              {t(`fleets.statement.kinds.${row.original.kind}`)}
              {row.original.is_reversal ? (
                <span className="text-muted-foreground ms-1 text-xs">
                  ({t("fleets.statement.reversal")})
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<FleetStatementLine>({
          id: "service",
          accessorFn: (row) => row.service?.service_no ?? "",
          labelKey: "fleets.statement.service",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.service ? (
              <Link
                href={routes.tenant.services.detail(
                  slug,
                  row.original.service.uuid,
                )}
                className="font-mono hover:underline"
                dir="ltr"
              >
                {row.original.service.service_no}
              </Link>
            ) : (
              "—"
            ),
        }),
        createColumn<FleetStatementLine>({
          id: "plate",
          accessorFn: (row) => row.service?.plate ?? "",
          labelKey: "fleets.fields.plate",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-mono" dir="ltr">
              {row.original.service?.plate ?? "—"}
            </span>
          ),
        }),
        createColumn<FleetStatementLine>({
          accessorKey: "description",
          labelKey: "fleets.statement.description",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.description ?? "—",
        }),
        createColumn<FleetStatementLine>({
          accessorKey: "debit",
          labelKey: "fleets.statement.debit",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums" dir="ltr">
              {Number(row.original.debit) ? money(row.original.debit) : "—"}
            </span>
          ),
        }),
        createColumn<FleetStatementLine>({
          accessorKey: "credit",
          labelKey: "fleets.statement.credit",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums" dir="ltr">
              {Number(row.original.credit) ? money(row.original.credit) : "—"}
            </span>
          ),
        }),
        createColumn<FleetStatementLine>({
          accessorKey: "balance",
          labelKey: "fleets.statement.balance",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums" dir="ltr">
              {money(row.original.balance)}
            </span>
          ),
        }),
      ] as ColumnDef<FleetStatementLine, unknown>[],
    // money closes over the statement currency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [format, slug, st?.currency, t],
  );

  return (
    <div className="space-y-4" data-testid="fleet-statement">
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="fleet-statement-from">
            {t("fleets.statement.period_from")}
          </Label>
          <DatePicker
            id="fleet-statement-from"
            value={from}
            onChange={setFrom}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="fleet-statement-to">
            {t("fleets.statement.period_to")}
          </Label>
          <DatePicker id="fleet-statement-to" value={to} onChange={setTo} />
        </div>
        <div className="flex flex-wrap gap-2">
          {FLEET_STATEMENT_EXPORT_FORMATS.map((f) => (
            <Button
              key={f}
              type="button"
              variant="outline"
              size="sm"
              disabled={!valid || !st || exporter.busy}
              data-testid={`fleet-statement-export-${f}`}
              onClick={() => exporter.start.mutate({ format: f, from, to })}
            >
              {exporter.busy &&
              (exporter.start.variables?.format === f ||
                exporter.current?.format === f) ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Download className="size-4" />
              )}
              {t(`exports.formats.${f}`)}
            </Button>
          ))}
        </div>
      </div>

      {statement.isError ? (
        <ErrorState
          title={
            isApiError(statement.error)
              ? statement.error.message
              : t("common.error_generic")
          }
          retryLabel={t("common.retry")}
          onRetry={() => void statement.refetch()}
        />
      ) : null}

      {st ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
          <Total
            label={t("fleets.statement.opening_balance")}
            value={money(st.opening_balance)}
          />
          <Total
            label={t("fleets.statement.service_income_total")}
            value={money(st.service_income_total)}
          />
          <Total
            label={t("fleets.statement.collection_total")}
            value={money(st.collection_total)}
          />
          <Total
            label={t("fleets.statement.service_count")}
            value={format.number(st.service_count)}
          />
          <Total
            label={t("fleets.statement.closing_balance")}
            value={money(st.closing_balance)}
          />
        </div>
      ) : null}

      <EntityTable
        columns={columns}
        data={st?.lines ?? []}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={statement.isLoading}
        emptyTitle={t("fleets.statement.empty_title")}
        emptyDescription={t("fleets.statement.empty_description")}
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        features={{ persistKey: FLEET_STATEMENT_PERSIST_KEY }}
      />
      {st ? (
        <p className="text-muted-foreground text-end text-sm">
          {t("fleets.statement.totals", {
            debit: money(st.debit_total),
            credit: money(st.credit_total),
          })}
        </p>
      ) : null}
    </div>
  );
}
