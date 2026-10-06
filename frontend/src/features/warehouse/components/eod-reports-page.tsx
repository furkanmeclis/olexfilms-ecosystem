"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, FileDown, RefreshCw, Sunset } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { downloadEodPdf } from "@/features/warehouse/components/eod-report-detail-page";
import { nativeSelectClass } from "@/features/warehouse/components/native-select-field";
import {
  enumFilterOptions,
  useWarehouseFilterOptions,
} from "@/features/warehouse/components/table-options";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  EOD_KINDS,
  EOD_SCOPES,
  eodScopeLabel,
  isFutureDay,
  todayIn,
} from "@/features/warehouse/lib/eod";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  warehouseKeys,
  warehouseService,
  type EodListQuery,
  type EodReport,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const EOD_PAGE_SIZE = 20;
export const EOD_PERSIST_KEY = "tenant-warehouse-eod-v1";
const SYSTEM = "system";

/**
 * Warehouse > End of day (TEC-207, TEC-376): stored daily reports (the
 * hourly cron writes yesterday's, kind auto) over a server DataTable —
 * sort (day, built at), day range, scope / warehouse / source filters, row
 * actions (open, PDF) and mobile cards — and a manual run for a day and
 * scope (system report or one warehouse, kind manual, replacing the
 * stored one).
 */
export function EodReportsPage({ slug }: { slug: string }) {
  const { t, format, locale } = useLocale();
  const router = useRouter();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const warehouseOptions = useWarehouseFilterOptions(access.allowed);

  const columns = useMemo(() => {
    const pdf = async (report: EodReport) => {
      try {
        await downloadEodPdf(
          report.uuid,
          locale,
          `eod-${report.report_date}.pdf`,
        );
        appToast.success(t("warehouse.eod.pdf_ready"));
      } catch {
        appToast.error(t("warehouse.eod.pdf_failed"));
      }
    };
    return [
      createColumn<EodReport>({
        accessorKey: "report_date",
        labelKey: "warehouse.eod.columns.date",
        enableSorting: true,
        gridPrimary: true,
        // report_date range → date_from / date_to (TEC-207).
        filterVariant: "date-range",
        param: "date",
        cell: ({ row }) => (
          <Link
            href={routes.tenant.warehouse.endOfDayReport(
              slug,
              row.original.uuid,
            )}
            className="font-medium hover:underline"
            data-testid="eod-row"
            data-uuid={row.original.uuid}
            onClick={(event) => event.stopPropagation()}
          >
            {format.date(`${row.original.report_date}T12:00:00Z`)}
          </Link>
        ),
      }),
      createColumn<EodReport>({
        id: "scope",
        accessorFn: (r) => (r.warehouse ? "warehouse" : "system"),
        labelKey: "warehouse.fields.scope",
        enableSorting: false,
        gridSecondary: true,
        filterVariant: "select",
        filterOptions: enumFilterOptions(
          EOD_SCOPES,
          "warehouse.eod.scope_filter",
        ),
        param: "scope",
        cell: ({ row }) =>
          eodScopeLabel(row.original) ?? t("warehouse.eod.system"),
      }),
      // Filter only: one warehouse's reports (warehouse_uuid, single).
      createColumn<EodReport>({
        id: "warehouse",
        accessorFn: (r) => r.warehouse?.uuid ?? "",
        labelKey: "warehouse.fields.warehouse",
        enableSorting: false,
        enableHiding: false,
        defaultHidden: true,
        filterVariant: "select",
        filterOptions: warehouseOptions,
        enableColumnFilter: warehouseOptions.length > 0,
        param: "warehouse_uuid",
        cell: () => null,
      }),
      createColumn<EodReport>({
        id: "movements",
        accessorFn: (r) => r.summary.totals.movement_count,
        labelKey: "warehouse.eod.columns.movements",
        enableSorting: false,
        cell: ({ row }) =>
          format.number(row.original.summary.totals.movement_count),
      }),
      createColumn<EodReport>({
        accessorKey: "kind",
        labelKey: "warehouse.eod.columns.kind",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(EOD_KINDS, "warehouse.eod.kind"),
        param: "kind",
        cell: ({ row }) => (
          <StatusChip
            label={t(`warehouse.eod.kind.${row.original.kind}`)}
            tone={row.original.kind === "manual" ? "warning" : "default"}
          />
        ),
      }),
      createColumn<EodReport>({
        accessorKey: "generated_at",
        labelKey: "warehouse.eod.columns.generated",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {format.dateTime(row.original.generated_at)}
          </span>
        ),
      }),
      createColumn<EodReport>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const report = row.original;
          const items: EntityRowAction[] = [
            {
              id: "view",
              label: t("common.view"),
              icon: Eye,
              onSelect: () =>
                router.push(
                  routes.tenant.warehouse.endOfDayReport(slug, report.uuid),
                ),
            },
            {
              id: "pdf",
              label: t("warehouse.eod.pdf"),
              icon: FileDown,
              onSelect: () => void pdf(report),
            },
          ];
          return <EntityRowActions actions={items} />;
        },
      }),
    ] as ColumnDef<EodReport, unknown>[];
  }, [format, locale, router, slug, t, warehouseOptions]);

  // Column meta drives the params: date (date_from/_to), scope /
  // warehouse_uuid (single), kind (CSV); sort report_date | generated_at.
  const listState = useServerListState({
    columns,
    initialSort: "-report_date",
    initialPageSize: EOD_PAGE_SIZE,
    persistKey: EOD_PERSIST_KEY,
  });
  const query: EodListQuery = listState.params;
  const list = useQuery({
    queryKey: warehouseKeys.eodReports(query),
    queryFn: () => warehouseService.listEodReports(query),
    enabled: access.allowed,
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.eod.title")}
      description={t("warehouse.eod.description")}
      icon={<Sunset className="size-6" />}
    >
      {canWrite ? <GenerateEodForm slug={slug} /> : null}

      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.warehouse.endOfDayReport(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("warehouse.eod.empty_title")}
        emptyDescription={t("warehouse.eod.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: EOD_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(r) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-medium">
                {format.date(`${r.report_date}T12:00:00Z`)}
              </span>
              <StatusChip
                label={t(`warehouse.eod.kind.${r.kind}`)}
                tone={r.kind === "manual" ? "warning" : "default"}
              />
            </div>
            <p className="text-sm">
              {eodScopeLabel(r) ?? t("warehouse.eod.system")}
            </p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>{format.number(r.summary.totals.movement_count)}</span>
              <span>{format.dateTime(r.generated_at)}</span>
            </div>
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
    </WarehouseShell>
  );
}

/** Manual run: a day (not in the future) and the system or one warehouse. */
export function GenerateEodForm({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const today = todayIn(format.timeZone);
  const [date, setDate] = useState(today);
  const [warehouseUuid, setWarehouseUuid] = useState(SYSTEM);
  const [error, setError] = useState<string | null>(null);

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });

  const generate = useMutation({
    mutationFn: () =>
      warehouseService.generateEodReport({
        date,
        warehouse_uuid: warehouseUuid === SYSTEM ? null : warehouseUuid,
      }),
    onSuccess: async (report) => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "eod-reports"] });
      appToast.success(t("warehouse.eod.generated"));
      router.push(routes.tenant.warehouse.endOfDayReport(slug, report.uuid));
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.eod.failed"))),
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) {
      setError(t("warehouse.validation.date"));
      return;
    }
    if (isFutureDay(date, today)) {
      setError(t("warehouse.validation.future_day"));
      return;
    }
    setError(null);
    generate.mutate();
  };

  return (
    <Card data-testid="eod-form">
      <CardHeader>
        <CardTitle>{t("warehouse.eod.generate_title")}</CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("warehouse.eod.generate_hint")}
        </p>
      </CardHeader>
      <CardContent>
        <form
          className="grid items-end gap-4 sm:grid-cols-3"
          onSubmit={onSubmit}
          noValidate
        >
          <div className="space-y-1.5">
            <Label htmlFor="eod-date">{t("warehouse.eod.columns.date")}</Label>
            <div data-testid="eod-date">
              <DatePicker id="eod-date" value={date} onChange={setDate} />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="eod-warehouse">{t("warehouse.fields.scope")}</Label>
            <select
              id="eod-warehouse"
              className={nativeSelectClass}
              value={warehouseUuid}
              onChange={(e) => setWarehouseUuid(e.target.value)}
              data-testid="eod-warehouse"
            >
              <option value={SYSTEM}>{t("warehouse.eod.system")}</option>
              {(warehouses.data?.items ?? []).map((w) => (
                <option key={w.uuid} value={w.uuid}>
                  {w.code} · {w.name}
                </option>
              ))}
            </select>
          </div>
          <Button
            type="submit"
            disabled={generate.isPending}
            data-testid="eod-generate"
          >
            <RefreshCw className="size-4" />
            {t("warehouse.eod.generate")}
          </Button>
          {error ? (
            <p
              role="alert"
              className="text-destructive text-sm sm:col-span-3"
              data-testid="eod-error"
            >
              {error}
            </p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  );
}
