"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ClipboardList, PackageSearch } from "lucide-react";
import Link from "next/link";
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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  adjustmentErrorMessage,
  INSPECTION_DECISIONS,
  RETURN_REASONS,
  type InspectionDecision,
} from "@/features/warehouse/lib/adjustments";
import {
  adjustmentKeys,
  adjustmentsService,
  type AdjustmentListQuery,
  type QuarantineItem,
} from "@/features/warehouse/services/adjustments.service";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const RETURNS_PERSIST_KEY = "tenant-warehouse-return-inspections-v1";

/**
 * Warehouse > Return inspection (TEC-1226): received returns wait in
 * quarantine (not counted, not pickable or usable). Per unit the warehouse
 * puts it back on stock (optionally into a bin), scraps it (a scrap
 * adjustment document) or sends it back to the supplier (stock.adjust);
 * the return reason is recorded. The quarantined total per product is
 * shown apart from the stock.
 */
export function ReturnInspectionsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const canAdjust = access.can(Permission.StockAdjust);
  const [deciding, setDeciding] = useState<QuarantineItem | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<QuarantineItem>({
          id: "barcode",
          accessorFn: (r) => r.unit.barcode,
          labelKey: "stock_adjustments.returns.columns.barcode",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium" dir="ltr">
              {row.original.unit.barcode}
            </span>
          ),
        }),
        createColumn<QuarantineItem>({
          id: "product",
          accessorFn: (r) => r.product.name,
          labelKey: "stock_adjustments.returns.columns.product",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div>
              {row.original.product.name}
              <div className="text-muted-foreground text-xs" dir="ltr">
                {row.original.product.sku}
                {row.original.remaining_meters
                  ? ` · ${row.original.remaining_meters} m`
                  : ""}
              </div>
            </div>
          ),
        }),
        createColumn<QuarantineItem>({
          id: "return",
          accessorFn: (r) => r.return?.transfer_no ?? "",
          labelKey: "stock_adjustments.returns.columns.return",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.return ? (
              <div>
                <Link
                  className="hover:underline"
                  dir="ltr"
                  href={routes.tenant.transfers.detail(
                    slug,
                    row.original.return.transfer_uuid,
                  )}
                  onClick={(e) => e.stopPropagation()}
                >
                  {row.original.return.transfer_no}
                </Link>
                <div className="text-muted-foreground text-xs">
                  {row.original.return.from_org_name}
                </div>
                {row.original.return.note ? (
                  <div className="text-muted-foreground text-xs">
                    {row.original.return.note}
                  </div>
                ) : null}
              </div>
            ) : (
              "—"
            ),
        }),
        createColumn<QuarantineItem>({
          id: "location",
          accessorFn: (r) => r.location?.full_code ?? "",
          labelKey: "stock_adjustments.returns.columns.location",
          enableSorting: false,
          cell: ({ row }) => (
            <span dir="ltr">{row.original.location?.full_code ?? "—"}</span>
          ),
        }),
        createColumn<QuarantineItem>({
          id: "quarantined_at",
          accessorFn: (r) => r.quarantined_at,
          labelKey: "stock_adjustments.returns.columns.since",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {format.dateTime(row.original.quarantined_at)}
            </span>
          ),
        }),
        createColumn<QuarantineItem>({
          id: "state",
          accessorFn: (r) => (r.pending_scrap ? "pending" : "waiting"),
          labelKey: "stock_adjustments.returns.columns.state",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(
                row.original.pending_scrap
                  ? "stock_adjustments.returns.pending_scrap"
                  : "stock_adjustments.returns.waiting",
              )}
              tone={row.original.pending_scrap ? "default" : "warning"}
            />
          ),
        }),
        createColumn<QuarantineItem>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            canWrite && !row.original.pending_scrap ? (
              <EntityRowActions
                actions={[
                  {
                    id: "inspect",
                    label: t("stock_adjustments.returns.inspect"),
                    icon: ClipboardList,
                    onSelect: () => setDeciding(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<QuarantineItem, unknown>[],
    [canWrite, format, slug, t],
  );

  const listState = useServerListState({
    columns,
    initialPageSize: 20,
    persistKey: RETURNS_PERSIST_KEY,
  });
  const query: AdjustmentListQuery = {
    q: listState.params.q,
    limit: listState.params.limit,
    offset: listState.params.offset,
  };
  const list = useQuery({
    queryKey: adjustmentKeys.quarantine(query),
    queryFn: () => adjustmentsService.quarantine(query),
    enabled: access.allowed,
  });
  const summary = useQuery({
    queryKey: adjustmentKeys.quarantineSummary,
    queryFn: () => adjustmentsService.quarantineSummary(),
    enabled: access.allowed,
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("stock_adjustments.returns.title")}
      description={t("stock_adjustments.returns.description")}
      icon={<PackageSearch className="size-6" />}
    >
      {(summary.data?.items.length ?? 0) > 0 ? (
        <Card data-testid="quarantine-summary">
          <CardHeader>
            <CardTitle>{t("stock_adjustments.returns.summary")}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
              {summary.data?.items.map((p) => (
                <li
                  key={p.product.uuid}
                  className="flex justify-between gap-2 rounded-md border p-2"
                >
                  <span>
                    {p.product.name}
                    <span
                      className="text-muted-foreground block text-xs"
                      dir="ltr"
                    >
                      {p.product.sku}
                    </span>
                  </span>
                  <span className="text-end tabular-nums">
                    {t("stock_adjustments.totals", {
                      pieces: p.units,
                      meters: p.meters,
                    })}
                  </span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : null}

      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.unit.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("stock_adjustments.returns.empty_title")}
        emptyDescription={t("stock_adjustments.returns.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: RETURNS_PERSIST_KEY, viewMode: true }}
        renderGridItem={(r) => (
          <div className="space-y-1">
            <div className="flex justify-between gap-2">
              <span className="font-medium" dir="ltr">
                {r.unit.barcode}
              </span>
              {canWrite && !r.pending_scrap ? (
                <Button size="sm" onClick={() => setDeciding(r)}>
                  {t("stock_adjustments.returns.inspect")}
                </Button>
              ) : null}
            </div>
            <p className="text-sm">{r.product.name}</p>
            <p className="text-muted-foreground text-xs">
              {r.return
                ? `${r.return.transfer_no} · ${r.return.from_org_name}`
                : "—"}
            </p>
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void list.refetch();
              void summary.refetch();
            }}
            refreshDisabled={list.isFetching}
          />
        }
      />

      {deciding ? (
        <InspectDialog
          item={deciding}
          canAdjust={canAdjust}
          onClose={() => setDeciding(null)}
        />
      ) : null}
    </WarehouseShell>
  );
}

function InspectDialog({
  item,
  canAdjust,
  onClose,
}: {
  item: QuarantineItem;
  canAdjust: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [decision, setDecision] = useState<InspectionDecision>("restock");
  const [reason, setReason] = useState<string>("");
  const [note, setNote] = useState("");
  const [warehouse, setWarehouse] = useState("");
  const [location, setLocation] = useState("");
  const [error, setError] = useState<string | null>(null);
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const active = (warehouses.data?.items ?? []).filter((w) => w.active);
  const decisions = INSPECTION_DECISIONS.filter(
    (d) => d !== "supplier_return" || canAdjust,
  );
  const inspect = useMutation({
    mutationFn: () =>
      adjustmentsService.inspect({
        unit_uuid: item.unit.uuid,
        decision,
        return_reason: reason as (typeof RETURN_REASONS)[number],
        note: note.trim() || null,
        location_uuid: decision === "restock" && location ? location : null,
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "quarantine"] });
      await qc.invalidateQueries({ queryKey: adjustmentKeys.all });
      appToast.success(t(`stock_adjustments.returns.done.${decision}`));
      onClose();
    },
    onError: (err) =>
      setError(adjustmentErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!reason) {
      setError(t("stock_adjustments.returns.reason_required"));
      return;
    }
    inspect.mutate();
  };
  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent>
        <form onSubmit={onSubmit} noValidate className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t("stock_adjustments.returns.inspect")}</DialogTitle>
            <DialogDescription>
              <span dir="ltr">{item.unit.barcode}</span> · {item.product.name}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <Label htmlFor="insp-decision">
              {t("stock_adjustments.returns.decision")}
            </Label>
            <Select
              value={decision}
              onValueChange={(v) => setDecision(v as InspectionDecision)}
            >
              <SelectTrigger id="insp-decision" data-testid="inspect-decision">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {decisions.map((d) => (
                  <SelectItem key={d} value={d}>
                    {t(`stock_adjustments.returns.decisions.${d}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-muted-foreground text-xs">
              {t(`stock_adjustments.returns.decision_help.${decision}`)}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="insp-reason">
              {t("stock_adjustments.returns.reason")}
            </Label>
            <Select value={reason} onValueChange={setReason}>
              <SelectTrigger id="insp-reason" data-testid="inspect-reason">
                <SelectValue
                  placeholder={t("stock_adjustments.returns.pick_reason")}
                />
              </SelectTrigger>
              <SelectContent>
                {RETURN_REASONS.map((r) => (
                  <SelectItem key={r} value={r}>
                    {t(`stock_adjustments.returns.reasons.${r}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {decision === "restock" ? (
            <div className="space-y-1.5">
              <Label htmlFor="insp-warehouse">
                {t("stock_adjustments.returns.bin")}
              </Label>
              <Select
                value={warehouse}
                onValueChange={(v) => {
                  setWarehouse(v);
                  setLocation("");
                }}
              >
                <SelectTrigger id="insp-warehouse">
                  <SelectValue
                    placeholder={t("stock_adjustments.returns.no_bin")}
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
              {warehouse ? (
                <LocationPicker
                  warehouseUuid={warehouse}
                  value={location}
                  onChange={setLocation}
                  testId="inspect"
                />
              ) : null}
            </div>
          ) : null}
          <div className="space-y-1.5">
            <Label htmlFor="insp-note">
              {t("stock_adjustments.fields.note")}
            </Label>
            <Textarea
              id="insp-note"
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
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={inspect.isPending}
              data-testid="inspect-save"
            >
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
