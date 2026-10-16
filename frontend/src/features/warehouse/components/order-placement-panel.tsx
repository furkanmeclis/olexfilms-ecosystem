"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { CheckCircle2, Info as InfoIcon, MapPin } from "lucide-react";
import { useMemo, useState } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Permission } from "@/config/permissions";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { useWarehouseAccess } from "@/features/warehouse/components/warehouse-shell";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  classifyUnitScan,
  selectedBarcodes,
  showOrderPlacement,
  unplacedUnits,
  type PlacementOrder,
} from "@/features/warehouse/lib/order-placement";
import { scanKind } from "@/features/warehouse/lib/scan";
import {
  warehouseKeys,
  warehouseService,
  type WarehouseOrderPlacementUnit,
} from "@/features/warehouse/services/warehouse.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const ORDER_PLACEMENT_PERSIST_KEY = "tenant-order-placement-units-v1";

type Target = { uuid: string; code: string | null };

/**
 * "Place on shelf" panel of a received order (TEC-765): the buyer center
 * or distributor shelves the received serial units onto one location of
 * its warehouses (POST /v1/warehouse/orders/{uuid}/place). Pick the
 * location or scan its QR, then scan or tick the units and place the
 * selected ones. Fixed barcodes are not placed in this step.
 */
export function OrderPlacementPanel({
  slug,
  order,
}: {
  slug: string;
  order: PlacementOrder & { uuid: string };
}) {
  const access = useWarehouseAccess(slug, Permission.WarehouseWrite);
  const visible = showOrderPlacement(
    order,
    access.orgType,
    access.can(Permission.WarehouseWrite),
  );
  if (!visible) return null;
  return <Panel orderUuid={order.uuid} />;
}

function Panel({ orderUuid }: { orderUuid: string }) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const key = warehouseKeys.orderPlacement(orderUuid);
  const [target, setTarget] = useState<Target | null>(null);
  const [warehouseUuid, setWarehouseUuid] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [scanError, setScanError] = useState<string | null>(null);
  const [placeError, setPlaceError] = useState<string | null>(null);

  const placement = useQuery({
    queryKey: key,
    queryFn: () => warehouseService.getOrderPlacement(orderUuid),
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const activeWarehouses = (warehouses.data?.items ?? []).filter(
    (w) => w.active,
  );
  const pickedWarehouse =
    warehouseUuid ||
    (activeWarehouses.length === 1 ? activeWarehouses[0].uuid : "");

  const p = placement.data;
  const waiting = useMemo(() => (p ? unplacedUnits(p) : []), [p]);

  const locationMessage = (err: unknown) =>
    isApiError(err) && err.status === 404
      ? t("warehouse.order_place.location_not_found")
      : warehouseErrorMessage(err, t, t("warehouse.order_place.failed"));

  const resolve = useMutation({
    mutationFn: (code: string) => warehouseService.scan(code),
    onSuccess: (res) => {
      if (res.type !== "location" || !res.location) {
        setScanError(t("warehouse.order_place.not_location"));
        return;
      }
      if (!res.location.active) {
        setScanError(t("warehouse.order_place.location_inactive"));
        return;
      }
      setScanError(null);
      setPlaceError(null);
      setTarget({ uuid: res.location.uuid, code: res.location.full_code });
    },
    onError: (err) => setScanError(locationMessage(err)),
  });

  const place = useMutation({
    mutationFn: (v: { location: string; barcodes: string[] }) =>
      warehouseService.placeOrder(orderUuid, {
        location_uuid: v.location,
        barcodes: v.barcodes,
      }),
    onSuccess: (res) => {
      setPlaceError(null);
      setSelected([]);
      const placed = res.units.filter((u) => !u.skipped).length;
      const skipped = res.units.length - placed;
      appToast.success(t("warehouse.order_place.done", { count: placed }));
      if (skipped > 0) {
        appToast.warning(
          t("warehouse.order_place.skipped", { count: skipped }),
        );
      }
      void qc.invalidateQueries({ queryKey: key });
    },
    onError: (err) => setPlaceError(locationMessage(err)),
  });

  const onLocationScan = (code: string) => {
    if (scanKind(code) === "unit") {
      setScanError(t("warehouse.order_place.scan_location_first"));
      return;
    }
    resolve.mutate(code);
  };

  const onUnitScan = (code: string) => {
    if (!p) return;
    if (scanKind(code) === "location") {
      onLocationScan(code);
      return;
    }
    const r = classifyUnitScan(p, code, selected);
    switch (r.kind) {
      case "select":
        setScanError(null);
        setSelected((s) => [...s, r.unit.unit_uuid]);
        break;
      case "already_selected":
        setScanError(
          t("warehouse.order_place.already_selected", {
            barcode: r.unit.barcode,
          }),
        );
        break;
      case "placed":
        setScanError(
          t("warehouse.order_place.already_placed", { barcode: r.barcode }),
        );
        break;
      case "fixed":
        setScanError(
          t("warehouse.order_place.fixed_scanned", { barcode: r.barcode }),
        );
        break;
      default:
        setScanError(
          t("warehouse.order_place.not_on_order", { barcode: r.barcode }),
        );
    }
  };

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<WarehouseOrderPlacementUnit>(),
        createColumn<WarehouseOrderPlacementUnit>({
          accessorKey: "barcode",
          labelKey: "warehouse.fields.barcode",
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className="font-mono text-xs"
              dir="ltr"
              data-testid="placement-unit"
              data-barcode={row.original.barcode}
            >
              {row.original.barcode}
            </span>
          ),
        }),
        createColumn<WarehouseOrderPlacementUnit>({
          id: "product",
          accessorFn: (u) => `${u.product.name} ${u.product.sku}`,
          labelKey: "warehouse.fields.product",
          gridSecondary: true,
          cell: ({ row }) => (
            <div>
              {row.original.product.name}
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product.sku}
              </div>
            </div>
          ),
        }),
        createColumn<WarehouseOrderPlacementUnit>({
          id: "meters",
          accessorFn: (u) => Number(u.meters ?? 0),
          labelKey: "warehouse.fields.meters",
          cell: ({ row }) =>
            row.original.meters ? (
              <span dir="ltr">
                {format.number(Number(row.original.meters))} m
              </span>
            ) : (
              "—"
            ),
        }),
      ] as ColumnDef<WarehouseOrderPlacementUnit, unknown>[],
    [format],
  );

  const rowSelection = useMemo<RowSelectionState>(
    () => Object.fromEntries(selected.map((id) => [id, true])),
    [selected],
  );

  // Forbidden / not the buyer: nothing to shelve here.
  if (placement.isError && isApiError(placement.error)) {
    if (placement.error.status === 403 || placement.error.status === 404) {
      return null;
    }
  }
  if (!p) {
    return placement.isError ? (
      <Card data-testid="order-placement">
        <CardContent className="text-destructive pt-6 text-sm">
          {t("common.error_generic")}
        </CardContent>
      </Card>
    ) : null;
  }
  if (p.units.length === 0 && p.fixed.length === 0) return null;

  const done = p.unplaced_count === 0;
  const barcodes = selectedBarcodes(p, selected);

  return (
    <Card data-testid="order-placement" data-done={done || undefined}>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2 text-base">
          <MapPin className="text-muted-foreground size-4" />
          {t("warehouse.order_place.title")}
          {done ? null : (
            <Badge variant="warning" data-testid="order-unplaced-badge">
              {t("warehouse.order_place.unplaced_badge", {
                count: p.unplaced_count,
              })}
            </Badge>
          )}
        </CardTitle>
        {done ? null : (
          <p className="text-muted-foreground text-sm">
            {t("warehouse.order_place.description")}
          </p>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {done ? (
          <div
            className="flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-400"
            data-testid="order-placement-done"
          >
            <CheckCircle2 className="size-4" />
            {p.units.length > 0
              ? t("warehouse.order_place.all_placed", {
                  count: p.placed_count,
                })
              : t("warehouse.order_place.nothing_to_place")}
          </div>
        ) : (
          <>
            <div className="grid gap-6 lg:grid-cols-2">
              <div className="space-y-3">
                <ScanInput
                  id="placement-location"
                  label={t("warehouse.order_place.scan_location")}
                  hint={t("warehouse.order_place.scan_location_hint")}
                  busy={resolve.isPending}
                  onScan={onLocationScan}
                />
                <div className="space-y-2">
                  <div className="space-y-1.5">
                    <Label htmlFor="placement-warehouse">
                      {t("warehouse.fields.warehouse")}
                    </Label>
                    <Select
                      value={pickedWarehouse}
                      onValueChange={(v) => {
                        setWarehouseUuid(v);
                        setTarget(null);
                      }}
                    >
                      <SelectTrigger
                        id="placement-warehouse"
                        data-testid="placement-warehouse"
                      >
                        <SelectValue
                          placeholder={t("warehouse.entries.pick_warehouse")}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {activeWarehouses.map((w) => (
                          <SelectItem key={w.uuid} value={w.uuid}>
                            {w.code} · {w.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <LocationPicker
                    key={pickedWarehouse}
                    warehouseUuid={pickedWarehouse}
                    value={target && target.code === null ? target.uuid : ""}
                    onChange={(uuid) => {
                      setPlaceError(null);
                      setTarget(uuid ? { uuid, code: null } : null);
                    }}
                    testId="placement-pick"
                  />
                </div>
                <p className="text-sm" data-testid="placement-target">
                  <span className="text-muted-foreground">
                    {t("warehouse.order_place.target")}:{" "}
                  </span>
                  {target ? (
                    target.code ? (
                      <span className="font-mono font-medium" dir="ltr">
                        {target.code}
                      </span>
                    ) : (
                      <span className="font-medium">
                        {t("warehouse.order_place.target_picked")}
                      </span>
                    )
                  ) : (
                    <span className="text-muted-foreground">
                      {t("warehouse.order_place.no_target")}
                    </span>
                  )}
                </p>
              </div>
              <div className="space-y-3">
                <ScanInput
                  id="placement-unit"
                  autoFocus={false}
                  label={t("warehouse.order_place.scan_unit")}
                  hint={t("warehouse.order_place.scan_unit_hint")}
                  onScan={onUnitScan}
                />
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setSelected(waiting.map((u) => u.unit_uuid))}
                    data-testid="placement-select-all"
                  >
                    {t("warehouse.order_place.select_all", {
                      count: waiting.length,
                    })}
                  </Button>
                  <Button
                    type="button"
                    disabled={
                      !target || barcodes.length === 0 || place.isPending
                    }
                    onClick={() =>
                      target &&
                      place.mutate({ location: target.uuid, barcodes })
                    }
                    data-testid="placement-submit"
                  >
                    <MapPin className="size-4" />
                    {t("warehouse.order_place.place_selected", {
                      count: barcodes.length,
                    })}
                  </Button>
                </div>
                {!target && barcodes.length > 0 ? (
                  <p className="text-muted-foreground text-xs">
                    {t("warehouse.order_place.pick_target_first")}
                  </p>
                ) : null}
              </div>
            </div>
            {scanError || placeError ? (
              <p
                role="alert"
                className="text-destructive text-sm"
                data-testid="placement-error"
              >
                {scanError ?? placeError}
              </p>
            ) : null}
            <EntityTable
              columns={columns}
              data={waiting}
              getRowId={(row) => row.unit_uuid}
              manual={CLIENT_SIDE_MANUAL}
              emptyTitle={t("warehouse.order_place.nothing_to_place")}
              emptyDescription=""
              initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
              pageSizeOptions={[20, 50, 100, 200]}
              state={{
                rowSelection,
                onRowSelectionChange: (updater) => {
                  const next =
                    typeof updater === "function"
                      ? updater(rowSelection)
                      : updater;
                  setSelected(
                    Object.keys(next).filter((id) => next[id] === true),
                  );
                },
              }}
              features={{
                persistKey: ORDER_PLACEMENT_PERSIST_KEY,
                rowSelection: true,
                pagination: waiting.length > 20,
              }}
            />
          </>
        )}
        {p.fixed.length > 0 ? (
          <div
            className="bg-muted/50 flex items-start gap-2 rounded-md border p-3 text-sm"
            data-testid="placement-fixed-note"
          >
            <InfoIcon className="text-muted-foreground mt-0.5 size-4 shrink-0" />
            <div className="space-y-0.5">
              <p className="font-medium">
                {t("warehouse.order_place.fixed_title", {
                  count: p.fixed.length,
                })}
              </p>
              <p className="text-muted-foreground">
                {t("warehouse.order_place.fixed_hint")}
              </p>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
