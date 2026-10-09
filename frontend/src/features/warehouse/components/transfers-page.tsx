"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  ArrowLeftRight,
  Eye,
  MoveRight,
  Plus,
  Truck,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import {
  enumFilterOptions,
  useWarehouseFilterOptions,
} from "@/features/warehouse/components/table-options";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  parseBarcodes,
  transferFormSchema,
  type TransferFormValues,
} from "@/features/warehouse/lib/forms";
import { scanKind, unitBarcode } from "@/features/warehouse/lib/scan";
import {
  TRANSFER_STATUSES,
  transferStatusTone,
} from "@/features/warehouse/lib/transfers";
import {
  warehouseKeys,
  warehouseService,
  type TransferListQuery,
  type Warehouse,
  type WarehouseMoveResult,
  type WarehouseTransfer,
} from "@/features/warehouse/services/warehouse.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const TRANSFER_PAGE_SIZE = 20;
export const TRANSFERS_PERSIST_KEY = "tenant-warehouse-transfers-v1";

type TransferRowAction = "ship" | "cancel";

/**
 * Warehouse > Transfers (TEC-205, TEC-376): a bin ↔ bin move inside a
 * warehouse in one step (scan units, scan the target bin) and the
 * warehouse ↔ warehouse transfer documents (draft → in transit →
 * completed, or cancelled) over a server DataTable — sort (no, created,
 * status), search (no, note), status / source / target / created filters,
 * row actions (open, ship a draft with lines, cancel) and mobile cards.
 */
export function TransfersPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const [creating, setCreating] = useState(false);
  const warehouseOptions = useWarehouseFilterOptions(access.allowed);

  const finish = useMutation({
    mutationFn: (v: { uuid: string; action: TransferRowAction }) =>
      v.action === "ship"
        ? warehouseService.shipTransfer(v.uuid)
        : warehouseService.cancelTransfer(v.uuid),
    onSuccess: async (next, v) => {
      qc.setQueryData(warehouseKeys.transfer(next.uuid), next);
      await qc.invalidateQueries({ queryKey: ["warehouse", "transfers"] });
      appToast.success(t(`warehouse.transfer.${v.action}_done`));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const runFinish = finish.mutate;
  const finishPending = finish.isPending;

  const columns = useMemo(() => {
    const ask = async (
      transfer: WarehouseTransfer,
      action: TransferRowAction,
    ) => {
      const ok = await confirm({
        title: t(`warehouse.transfer.${action}_title`),
        description: t(`warehouse.transfer.${action}_description`),
        confirmLabel: t(`warehouse.transfer.${action}`),
        variant: action === "cancel" ? "destructive" : "default",
      });
      if (ok) runFinish({ uuid: transfer.uuid, action });
    };
    return [
      createColumn<WarehouseTransfer>({
        accessorKey: "transfer_no",
        labelKey: "warehouse.transfers.columns.no",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0">
            <Link
              href={routes.tenant.warehouse.transfer(slug, row.original.uuid)}
              className="font-mono font-medium hover:underline"
              dir="ltr"
              data-testid="transfer-row"
              data-uuid={row.original.uuid}
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.transfer_no}
            </Link>
            {row.original.note ? (
              <div className="text-muted-foreground truncate text-xs">
                {row.original.note}
              </div>
            ) : null}
          </div>
        ),
      }),
      createColumn<WarehouseTransfer>({
        id: "from_warehouse",
        accessorFn: (r) => r.from_warehouse.code,
        labelKey: "warehouse.fields.from_warehouse",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: warehouseOptions,
        enableColumnFilter: warehouseOptions.length > 0,
        param: "from_warehouse_uuid",
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="inline-flex items-center gap-1">
            {row.original.from_warehouse.code}
            <MoveRight className="text-muted-foreground size-3 rtl:rotate-180" />
          </span>
        ),
      }),
      createColumn<WarehouseTransfer>({
        id: "to_warehouse",
        accessorFn: (r) => r.to_warehouse.code,
        labelKey: "warehouse.fields.to_warehouse",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: warehouseOptions,
        enableColumnFilter: warehouseOptions.length > 0,
        param: "to_warehouse_uuid",
        cell: ({ row }) => row.original.to_warehouse.code,
      }),
      createColumn<WarehouseTransfer>({
        accessorKey: "status",
        labelKey: "warehouse.entries.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(
          TRANSFER_STATUSES,
          "warehouse.transfer_status",
        ),
        param: "status",
        cell: ({ row }) => (
          <StatusChip
            label={t(`warehouse.transfer_status.${row.original.status}`)}
            tone={transferStatusTone(row.original.status)}
          />
        ),
      }),
      createColumn<WarehouseTransfer>({
        accessorKey: "line_count",
        labelKey: "warehouse.entries.columns.lines",
        enableSorting: false,
        cell: ({ row }) => format.number(row.original.line_count),
      }),
      createColumn<WarehouseTransfer>({
        accessorKey: "created_at",
        labelKey: "warehouse.entries.columns.created",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
      createColumn<WarehouseTransfer>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const transfer = row.original;
          const items: EntityRowAction[] = [
            {
              id: "view",
              label: t("common.view"),
              icon: Eye,
              onSelect: () =>
                router.push(
                  routes.tenant.warehouse.transfer(slug, transfer.uuid),
                ),
            },
          ];
          // Receiving needs target locations: it stays on the detail page.
          if (
            canWrite &&
            transfer.status === "draft" &&
            transfer.line_count > 0
          ) {
            items.push({
              id: "ship",
              label: t("warehouse.transfer.ship"),
              icon: Truck,
              disabled: finishPending,
              onSelect: () => void ask(transfer, "ship"),
            });
          }
          if (
            canWrite &&
            (transfer.status === "draft" || transfer.status === "in_transit")
          ) {
            items.push({
              id: "cancel",
              label: t("warehouse.transfer.cancel"),
              icon: XCircle,
              variant: "destructive",
              disabled: finishPending,
              onSelect: () => void ask(transfer, "cancel"),
            });
          }
          return <EntityRowActions actions={items} />;
        },
      }),
    ] as ColumnDef<WarehouseTransfer, unknown>[];
  }, [
    canWrite,
    confirm,
    finishPending,
    format,
    router,
    runFinish,
    slug,
    t,
    warehouseOptions,
  ]);

  // Column meta drives the params: from_/to_warehouse_uuid / status (CSV),
  // created (created_from/_to); sort transfer_no | created_at | status.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: TRANSFER_PAGE_SIZE,
    persistKey: TRANSFERS_PERSIST_KEY,
  });
  const query: TransferListQuery = listState.params;
  const list = useQuery({
    queryKey: warehouseKeys.transfers(query),
    queryFn: () => warehouseService.listTransfers(query),
    enabled: access.allowed,
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.transfers.title")}
      description={t("warehouse.transfers.description")}
      icon={<ArrowLeftRight className="size-6" />}
      actions={
        canWrite && !creating ? (
          <Button
            type="button"
            onClick={() => setCreating(true)}
            data-testid="transfer-new"
          >
            <Plus className="size-4" />
            {t("warehouse.transfers.new")}
          </Button>
        ) : null
      }
    >
      {creating ? (
        <NewTransferForm slug={slug} onCancel={() => setCreating(false)} />
      ) : null}

      {canWrite ? <BinMoveCard /> : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("warehouse.transfers.list_title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <EntityTable
            columns={columns}
            data={list.data?.items ?? []}
            getRowId={(row) => row.uuid}
            onRowClick={(row) =>
              router.push(routes.tenant.warehouse.transfer(slug, row.uuid))
            }
            isLoading={list.isLoading}
            isError={list.isError}
            onRetry={() => void list.refetch()}
            emptyTitle={t("warehouse.transfers.empty_title")}
            emptyDescription={t("warehouse.transfers.empty_description")}
            rowCount={list.data?.total ?? 0}
            state={listState.tableState}
            features={{
              persistKey: TRANSFERS_PERSIST_KEY,
              rowSelection: false,
              viewMode: true,
            }}
            renderGridItem={(r) => (
              <div className="space-y-2">
                <div className="flex items-start justify-between gap-2">
                  <span className="font-mono text-sm font-semibold" dir="ltr">
                    {r.transfer_no}
                  </span>
                  <StatusChip
                    label={t(`warehouse.transfer_status.${r.status}`)}
                    tone={transferStatusTone(r.status)}
                  />
                </div>
                <p className="inline-flex items-center gap-1 text-sm">
                  {r.from_warehouse.code}
                  <MoveRight className="size-3 rtl:rotate-180" />
                  {r.to_warehouse.code}
                </p>
                <div className="text-muted-foreground flex justify-between gap-2 text-xs">
                  <span>{format.number(r.line_count)}</span>
                  <span>{format.dateTime(r.created_at)}</span>
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
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}

/** Draft warehouse transfer: source, target (different) and a note. */
export function NewTransferForm({
  slug,
  onCancel,
}: {
  slug: string;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const active: Warehouse[] = (warehouses.data?.items ?? []).filter(
    (w) => w.active,
  );
  const options = active.map((w) => ({
    value: w.uuid,
    label: `${w.code} · ${w.name}`,
  }));

  const create = useMutation({
    mutationFn: (v: TransferFormValues) =>
      warehouseService.createTransfer({
        from_warehouse_uuid: v.from_warehouse_uuid,
        to_warehouse_uuid: v.to_warehouse_uuid,
        note: v.note || null,
      }),
    onSuccess: async (transfer) => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "transfers"] });
      router.push(routes.tenant.warehouse.transfer(slug, transfer.uuid));
    },
  });

  return (
    <Card data-testid="transfer-form">
      <CardHeader>
        <CardTitle>{t("warehouse.transfers.new_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {warehouses.isSuccess && active.length < 2 ? (
          <p
            className="text-muted-foreground mb-4 text-sm"
            data-testid="transfer-need-two"
          >
            {t("warehouse.transfers.need_two_warehouses")}
          </p>
        ) : null}
        <AppForm<TransferFormValues>
          schema={transferFormSchema(t) as never}
          defaultValues={{
            from_warehouse_uuid: "",
            to_warehouse_uuid: "",
            note: "",
          }}
          onSubmit={async (v) => {
            setError(null);
            try {
              await create.mutateAsync(v);
            } catch (err) {
              setError(
                warehouseErrorMessage(err, t, t("warehouse.form.error")),
              );
            }
          }}
          className="space-y-4"
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <AppSelect
              name="from_warehouse_uuid"
              label={t("warehouse.fields.from_warehouse")}
              placeholder={t("warehouse.entries.pick_warehouse")}
              options={options}
              testId="transfer-from"
            />
            <AppSelect
              name="to_warehouse_uuid"
              label={t("warehouse.fields.to_warehouse")}
              placeholder={t("warehouse.entries.pick_warehouse")}
              options={options}
              testId="transfer-to"
            />
          </div>
          <AppInput
            name="note"
            label={t("warehouse.fields.note")}
            data-testid="transfer-note"
          />
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("warehouse.form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={create.isPending}
              data-testid="transfer-create"
            >
              {t("warehouse.transfers.create")}
            </Button>
          </div>
        </AppForm>
      </CardContent>
    </Card>
  );
}

/**
 * Bin ↔ bin move (POST /v1/warehouse/moves): collect unit barcodes, then
 * scan the target location QR; every unit gets one placement. Moving to
 * another warehouse is a warehouse transfer (the backend answers 400).
 */
export function BinMoveCard() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<WarehouseMoveResult | null>(null);
  const barcodes = parseBarcodes(text).map(unitBarcode);

  const move = useMutation({
    mutationFn: (locationCode: string) =>
      warehouseService.move({ barcodes, location_code: locationCode }),
    onSuccess: (res) => {
      setError(null);
      setResult(res);
      setText("");
      void qc.invalidateQueries({ queryKey: ["warehouse"] });
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.move.failed"))),
  });

  const onLocation = (code: string) => {
    if (scanKind(code) === "unit") {
      setText((prev) => (prev ? `${prev}\n${code}` : code));
      return;
    }
    if (barcodes.length === 0) {
      setError(t("warehouse.move.no_units"));
      return;
    }
    move.mutate(code);
  };

  return (
    <Card data-testid="bin-move">
      <CardHeader>
        <CardTitle>{t("warehouse.move.title")}</CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("warehouse.move.description")}
        </p>
      </CardHeader>
      <CardContent className="grid gap-6 lg:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="move-units">{t("warehouse.move.units")}</Label>
          <Textarea
            id="move-units"
            value={text}
            rows={4}
            dir="ltr"
            className="font-mono"
            onChange={(e) => setText(e.target.value)}
            data-testid="move-units"
          />
          <p className="text-muted-foreground text-xs">
            {t("warehouse.move.units_hint", { count: barcodes.length })}
          </p>
        </div>
        <ScanInput
          id="move-location"
          autoFocus={false}
          label={t("warehouse.entry.scan_location")}
          hint={t("warehouse.move.location_hint")}
          busy={move.isPending}
          onScan={onLocation}
        />
        {error ? (
          <p
            role="alert"
            className="text-destructive text-sm lg:col-span-2"
            data-testid="move-error"
          >
            {error}
          </p>
        ) : null}
        {result ? (
          <p
            className="text-sm lg:col-span-2"
            role="status"
            data-testid="move-result"
          >
            {t("warehouse.move.done", {
              count: result.units.length,
              location: result.location.full_code ?? result.location.code,
            })}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
