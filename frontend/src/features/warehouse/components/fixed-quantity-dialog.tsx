"use client";

import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

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
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  warehouseService,
  type WarehouseScanResult,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export type FixedSelection = {
  barcode: string;
  quantity: number;
  source_location_uuid: string;
};

type FixedUnit = {
  barcode: string;
  product: NonNullable<WarehouseScanResult["product"]>;
  warehouseUuid: string;
};

export function FixedQuantityDialog({
  unit,
  action,
  onConfirm,
  onCancel,
}: {
  unit: FixedUnit | null;
  action: "add" | "move";
  onConfirm: (selection: FixedSelection) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [locationUuid, setLocationUuid] = useState("");
  const [quantity, setQuantity] = useState("");
  const [sourceError, setSourceError] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);
  const [checking, setChecking] = useState(false);

  const stocks = useQuery({
    queryKey: [
      "warehouse",
      "fixed-sources",
      unit?.warehouseUuid,
      unit?.product.uuid,
      unit?.barcode,
    ],
    enabled: Boolean(unit),
    queryFn: async () => {
      if (!unit) return [];
      const rooms = await warehouseService.listRooms(unit.warehouseUuid);
      const groups = await Promise.all(
        rooms.items.map((room) => warehouseService.listLocations(room.uuid)),
      );
      const bins = groups
        .flatMap((group) => group.items)
        .filter((loc) => loc.type === "bin" && loc.active);
      const rows = await Promise.all(
        bins.map(async (loc) => {
          const page = await warehouseService.listLocationProducts(
            loc.uuid,
            unit.product.uuid,
          );
          const holding = page.items.find(
            (row) => row.product.uuid === unit.product.uuid,
          );
          const available =
            holding?.fixed_barcodes.find(
              (fixed) => fixed.barcode === unit.barcode,
            )?.quantity ?? 0;
          return { uuid: loc.uuid, label: loc.full_code, available };
        }),
      );
      return rows.filter((row) => row.available > 0);
    },
  });

  const selected = stocks.data?.find((row) => row.uuid === locationUuid);
  const count = Number(quantity);
  const invalidQuantity =
    quantity !== "" && (!Number.isInteger(count) || count < 1);
  const exceeds = Boolean(
    selected && Number.isInteger(count) && count > selected.available,
  );
  const quantityError = invalidQuantity
    ? t("warehouse.fixed.quantity_invalid")
    : exceeds
      ? t("warehouse.fixed.quantity_exceeds", {
          count: selected?.available ?? 0,
        })
      : submitted && !quantity
        ? t("warehouse.fixed.quantity_invalid")
        : null;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setSubmitted(true);
    if (
      !unit ||
      !selected ||
      !Number.isInteger(count) ||
      count < 1 ||
      count > selected.available
    )
      return;
    onConfirm({
      barcode: unit.barcode,
      quantity: count,
      source_location_uuid: selected.uuid,
    });
  };

  const scanSource = async (code: string) => {
    setChecking(true);
    setSourceError(null);
    try {
      const result = await warehouseService.scan(code);
      if (result.type !== "location" || result.location?.type !== "bin") {
        setSourceError(t("warehouse.fixed.source_invalid"));
        return;
      }
      const match = stocks.data?.find(
        (row) => row.uuid === result.location?.uuid,
      );
      if (!match) {
        setSourceError(t("warehouse.fixed.source_unavailable"));
        return;
      }
      setLocationUuid(match.uuid);
    } catch (error) {
      setSourceError(
        warehouseErrorMessage(error, t, t("warehouse.fixed.source_invalid")),
      );
    } finally {
      setChecking(false);
    }
  };

  return (
    <Dialog
      open={Boolean(unit)}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      <DialogContent data-testid="fixed-quantity-dialog" key={unit?.barcode}>
        <DialogHeader>
          <DialogTitle>
            {unit?.product.name} · {unit?.barcode}
          </DialogTitle>
          <DialogDescription>
            {t("warehouse.fixed.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="fixed-source">
            {t("warehouse.fixed.source_label")}
          </Label>
          <select
            id="fixed-source"
            data-testid="fixed-source"
            className="border-input bg-background h-10 w-full rounded-md border px-3"
            value={locationUuid}
            onChange={(event) => {
              setLocationUuid(event.target.value);
              setSourceError(null);
            }}
          >
            <option value="">{t("warehouse.fixed.source_select")}</option>
            {(stocks.data ?? []).map((row) => (
              <option key={row.uuid} value={row.uuid}>
                {row.label} ·{" "}
                {t("warehouse.fixed.available", { count: row.available })}
              </option>
            ))}
          </select>
          {stocks.isPending ? (
            <p className="text-muted-foreground text-xs">
              {t("warehouse.fixed.loading")}
            </p>
          ) : null}
          {stocks.isError ? (
            <p role="alert" className="text-destructive text-xs">
              {t("warehouse.fixed.load_failed")}
            </p>
          ) : null}
          {submitted && !selected ? (
            <p role="alert" className="text-destructive text-xs">
              {t("warehouse.fixed.source_invalid")}
            </p>
          ) : null}
        </div>
        <ScanInput
          id="fixed-source-scan"
          expect="bin"
          autoFocus={false}
          busy={checking}
          onScan={scanSource}
          label={t("warehouse.fixed.source_scan")}
        />
        {sourceError ? (
          <p role="alert" className="text-destructive text-xs">
            {sourceError}
          </p>
        ) : null}
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="fixed-quantity">
              {t("warehouse.fixed.quantity_label")}
            </Label>
            <Input
              id="fixed-quantity"
              data-testid="fixed-quantity"
              type="number"
              min={1}
              max={selected?.available}
              step={1}
              value={quantity}
              onChange={(event) => setQuantity(event.target.value)}
              aria-invalid={Boolean(quantityError)}
            />
            {selected ? (
              <p className="text-muted-foreground text-xs">
                {t("warehouse.fixed.available", { count: selected.available })}
              </p>
            ) : null}
            {quantityError ? (
              <p
                role="alert"
                data-testid="fixed-quantity-error"
                className="text-destructive text-xs"
              >
                {quantityError}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" data-testid="fixed-confirm">
              {t(`warehouse.fixed.${action}`)}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
