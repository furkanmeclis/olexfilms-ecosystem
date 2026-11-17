"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, FilePenLine, Plus, Settings2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  enumFilterOptions,
  useWarehouseFilterOptions,
} from "@/features/warehouse/components/table-options";
import {
  listExportQuery,
  useExportSelection,
  WarehouseExportMenu,
} from "@/features/warehouse/components/warehouse-export";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  ADJUSTMENT_SOURCES,
  ADJUSTMENT_STATUSES,
  ADJUSTMENT_TYPES,
  adjustmentErrorMessage,
  adjustmentStatusTone,
  normalizeMeters,
} from "@/features/warehouse/lib/adjustments";
import {
  ADJUSTMENTS_EXPORT_PATH,
  adjustmentKeys,
  adjustmentsService,
  type AdjustmentListQuery,
  type StockAdjustment,
  type StockAdjustmentSettings,
} from "@/features/warehouse/services/adjustments.service";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const ADJUSTMENTS_PAGE_SIZE = 20;
export const ADJUSTMENTS_PERSIST_KEY = "tenant-warehouse-adjustments-v1";

/**
 * Warehouse > Stock adjustments (TEC-1226): waste, damage, scrap, loss,
 * found and other documents over a server DataTable (sort, search, status /
 * type / source / warehouse / created filters, selection export, mobile
 * cards), a new draft and the second approval thresholds. No price.
 */
export function AdjustmentsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const canAdjust = access.can(Permission.StockAdjust);
  const [creating, setCreating] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const warehouseOptions = useWarehouseFilterOptions(access.allowed);

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<StockAdjustment>(),
        createColumn<StockAdjustment>({
          accessorKey: "adjustment_no",
          labelKey: "stock_adjustments.columns.number",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.warehouse.adjustment(slug, row.original.uuid)}
              className="font-medium hover:underline"
              data-testid="adjustment-row"
              onClick={(event) => event.stopPropagation()}
              dir="ltr"
            >
              {row.original.adjustment_no}
            </Link>
          ),
        }),
        createColumn<StockAdjustment>({
          accessorKey: "type",
          labelKey: "stock_adjustments.columns.type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            ADJUSTMENT_TYPES,
            "stock_adjustments.type",
          ),
          param: "type",
          gridSecondary: true,
          cell: ({ row }) => t(`stock_adjustments.type.${row.original.type}`),
        }),
        createColumn<StockAdjustment>({
          accessorKey: "status",
          labelKey: "stock_adjustments.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            ADJUSTMENT_STATUSES,
            "stock_adjustments.status",
          ),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`stock_adjustments.status.${row.original.status}`)}
              tone={adjustmentStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StockAdjustment>({
          accessorKey: "source",
          labelKey: "stock_adjustments.columns.source",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            ADJUSTMENT_SOURCES,
            "stock_adjustments.source",
          ),
          param: "source",
          cell: ({ row }) =>
            t(`stock_adjustments.source.${row.original.source}`),
        }),
        createColumn<StockAdjustment>({
          id: "warehouse",
          accessorFn: (a) => a.warehouse?.name ?? "",
          labelKey: "stock_adjustments.columns.warehouse",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: warehouseOptions,
          enableColumnFilter: warehouseOptions.length > 0,
          param: "warehouse_uuid",
          cell: ({ row }) =>
            row.original.warehouse
              ? `${row.original.warehouse.code} · ${row.original.warehouse.name}`
              : "—",
        }),
        createColumn<StockAdjustment>({
          accessorKey: "total_quantity",
          labelKey: "stock_adjustments.columns.pieces",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(row.original.total_quantity)}
            </span>
          ),
        }),
        createColumn<StockAdjustment>({
          accessorKey: "total_meters",
          labelKey: "stock_adjustments.columns.meters",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums" dir="ltr">
              {row.original.total_meters}
            </span>
          ),
        }),
        createColumn<StockAdjustment>({
          accessorKey: "created_at",
          labelKey: "stock_adjustments.columns.created",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<StockAdjustment>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("common.view"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.warehouse.adjustment(
                        slug,
                        row.original.uuid,
                      ),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<StockAdjustment, unknown>[],
    [format, router, slug, t, warehouseOptions],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: ADJUSTMENTS_PAGE_SIZE,
    persistKey: ADJUSTMENTS_PERSIST_KEY,
  });
  const query: AdjustmentListQuery = listState.params;
  const selection = useExportSelection();
  const exportQuery = listExportQuery(
    listState.filterParams,
    listState.params,
    selection.selected,
  );
  const list = useQuery({
    queryKey: adjustmentKeys.list(query),
    queryFn: () => adjustmentsService.list(query),
    enabled: access.allowed,
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("stock_adjustments.title")}
      description={t("stock_adjustments.description")}
      icon={<FilePenLine className="size-6" />}
      actions={
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => setSettingsOpen((v) => !v)}
            data-testid="adjustment-settings-toggle"
          >
            <Settings2 className="size-4" />
            {t("stock_adjustments.settings.title")}
          </Button>
          {canWrite && !creating ? (
            <Button
              type="button"
              onClick={() => setCreating(true)}
              data-testid="adjustment-new"
            >
              <Plus className="size-4" />
              {t("stock_adjustments.new")}
            </Button>
          ) : null}
        </div>
      }
    >
      {settingsOpen ? <ThresholdSettings canEdit={canAdjust} /> : null}
      {creating ? (
        <NewAdjustmentForm slug={slug} onCancel={() => setCreating(false)} />
      ) : null}

      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.warehouse.adjustment(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("stock_adjustments.empty_title")}
        emptyDescription={t("stock_adjustments.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={{
          ...listState.tableState,
          rowSelection: selection.rowSelection,
          onRowSelectionChange: selection.onRowSelectionChange,
        }}
        features={{
          persistKey: ADJUSTMENTS_PERSIST_KEY,
          rowSelection: true,
          viewMode: true,
        }}
        renderGridItem={(a) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-medium" dir="ltr">
                {a.adjustment_no}
              </span>
              <StatusChip
                label={t(`stock_adjustments.status.${a.status}`)}
                tone={adjustmentStatusTone(a.status)}
              />
            </div>
            <p className="text-sm">
              {t(`stock_adjustments.type.${a.type}`)} ·{" "}
              {t(`stock_adjustments.source.${a.source}`)}
            </p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>
                {t("stock_adjustments.totals", {
                  pieces: a.total_quantity,
                  meters: a.total_meters,
                })}
              </span>
              <span>{format.dateTime(a.created_at)}</span>
            </div>
          </div>
        )}
        toolbarExtra={
          <>
            <WarehouseExportMenu
              slug={slug}
              exportPath={ADJUSTMENTS_EXPORT_PATH}
              query={exportQuery}
              testId="adjustments-export"
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </WarehouseShell>
  );
}

/** Second approval thresholds (pieces and meters; stock.adjust edits). */
function ThresholdSettings({ canEdit }: { canEdit: boolean }) {
  const settings = useQuery({
    queryKey: adjustmentKeys.settings,
    queryFn: () => adjustmentsService.settings(),
  });
  if (!settings.data) return null;
  return (
    <ThresholdForm
      key={`${settings.data.threshold_quantity}-${settings.data.threshold_meters}`}
      initial={settings.data}
      canEdit={canEdit}
    />
  );
}

function ThresholdForm({
  initial,
  canEdit,
}: {
  initial: StockAdjustmentSettings;
  canEdit: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [qty, setQty] = useState(String(initial.threshold_quantity));
  const [meters, setMeters] = useState(initial.threshold_meters);
  const [error, setError] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () =>
      adjustmentsService.saveSettings({
        threshold_quantity: Number(qty),
        threshold_meters: normalizeMeters(meters) ?? "0",
      }),
    onSuccess: (next) => {
      qc.setQueryData(adjustmentKeys.settings, next);
      appToast.success(t("stock_adjustments.settings.saved"));
    },
    onError: (err) =>
      setError(adjustmentErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    const q = Number(qty);
    if (!Number.isInteger(q) || q < 0) {
      setError(t("stock_adjustments.settings.invalid"));
      return;
    }
    if (meters.trim() !== "0" && !normalizeMeters(meters)) {
      setError(t("stock_adjustments.settings.invalid"));
      return;
    }
    save.mutate();
  };
  return (
    <Card data-testid="adjustment-settings">
      <CardHeader>
        <CardTitle>{t("stock_adjustments.settings.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={onSubmit} noValidate>
          <p className="text-muted-foreground text-sm">
            {t("stock_adjustments.settings.help")}
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="adj-threshold-qty">
                {t("stock_adjustments.settings.quantity")}
              </Label>
              <Input
                id="adj-threshold-qty"
                inputMode="numeric"
                value={qty}
                disabled={!canEdit}
                onChange={(e) => setQty(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="adj-threshold-meters">
                {t("stock_adjustments.settings.meters")}
              </Label>
              <Input
                id="adj-threshold-meters"
                inputMode="decimal"
                value={meters}
                disabled={!canEdit}
                onChange={(e) => setMeters(e.target.value)}
              />
            </div>
          </div>
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          {canEdit ? (
            <Button type="submit" disabled={save.isPending}>
              {t("common.save")}
            </Button>
          ) : (
            <p className="text-muted-foreground text-xs">
              {t("stock_adjustments.settings.read_only")}
            </p>
          )}
        </form>
      </CardContent>
    </Card>
  );
}

/** New draft: type, reason code, note and an optional warehouse. */
function NewAdjustmentForm({
  slug,
  onCancel,
}: {
  slug: string;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [type, setType] = useState<string>("");
  const [reason, setReason] = useState("");
  const [note, setNote] = useState("");
  const [warehouse, setWarehouse] = useState("");
  const [error, setError] = useState<string | null>(null);
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const active = (warehouses.data?.items ?? []).filter((w) => w.active);
  const create = useMutation({
    mutationFn: () =>
      adjustmentsService.create({
        type: type as StockAdjustment["type"],
        reason_code: reason.trim() || null,
        note: note.trim() || null,
        warehouse_uuid: warehouse || null,
      }),
    onSuccess: async (a) => {
      await qc.invalidateQueries({ queryKey: adjustmentKeys.all });
      router.push(routes.tenant.warehouse.adjustment(slug, a.uuid));
    },
    onError: (err) =>
      setError(adjustmentErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!type) {
      setError(t("stock_adjustments.form.type_required"));
      return;
    }
    create.mutate();
  };
  return (
    <Card data-testid="adjustment-form">
      <CardHeader>
        <CardTitle>{t("stock_adjustments.new")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={onSubmit} noValidate>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-1.5">
              <Label htmlFor="adj-type">
                {t("stock_adjustments.columns.type")}
              </Label>
              <Select value={type} onValueChange={setType}>
                <SelectTrigger id="adj-type" data-testid="adjustment-type">
                  <SelectValue
                    placeholder={t("stock_adjustments.form.pick_type")}
                  />
                </SelectTrigger>
                <SelectContent>
                  {ADJUSTMENT_TYPES.map((v) => (
                    <SelectItem key={v} value={v}>
                      {t(`stock_adjustments.type.${v}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="adj-reason">
                {t("stock_adjustments.fields.reason_code")}
              </Label>
              <Input
                id="adj-reason"
                maxLength={64}
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="adj-warehouse">
                {t("stock_adjustments.columns.warehouse")}
              </Label>
              <Select value={warehouse} onValueChange={setWarehouse}>
                <SelectTrigger id="adj-warehouse">
                  <SelectValue
                    placeholder={t("stock_adjustments.form.any_warehouse")}
                  />
                </SelectTrigger>
                <SelectContent>
                  {active.map((w) => (
                    <SelectItem key={w.uuid} value={w.uuid}>
                      {w.code} · {w.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="adj-note">
              {t("stock_adjustments.fields.note")}
            </Label>
            <Textarea
              id="adj-note"
              maxLength={1000}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          <div className="flex gap-2">
            <Button type="submit" disabled={create.isPending}>
              {t("stock_adjustments.form.create")}
            </Button>
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common.cancel")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
