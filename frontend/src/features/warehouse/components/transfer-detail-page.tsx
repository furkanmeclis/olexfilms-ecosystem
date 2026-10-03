"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeftRight,
  CheckCircle2,
  MapPin,
  Trash2,
  Truck,
  XCircle,
} from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Info } from "@/features/warehouse/components/list-controls";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import { scanKind, unitBarcode } from "@/features/warehouse/lib/scan";
import {
  canCancel,
  canCompleteWithoutLocation,
  canEditLines,
  canPlace,
  canShip,
  transferStatusTone,
  untargetedLines,
} from "@/features/warehouse/lib/transfers";
import {
  warehouseKeys,
  warehouseService,
  type WarehouseTransfer,
} from "@/features/warehouse/services/warehouse.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Finish = "ship" | "complete" | "cancel";

/**
 * One warehouse transfer (TEC-205). Draft: scan units of the source
 * warehouse, optionally set target locations, ship (transfer_out). In
 * transit: set or scan the target locations and receive (transfer_in +
 * placement). Draft and in transit can be cancelled (units go back).
 */
export function TransferDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const [locationUuid, setLocationUuid] = useState("");
  const [lineError, setLineError] = useState<string | null>(null);
  const [placeError, setPlaceError] = useState<string | null>(null);

  const key = warehouseKeys.transfer(uuid);
  const transferQuery = useQuery({
    queryKey: key,
    queryFn: () => warehouseService.getTransfer(uuid),
    enabled: access.allowed,
  });
  const transfer = transferQuery.data;

  const apply = (next: WarehouseTransfer) => {
    qc.setQueryData(key, next);
    void qc.invalidateQueries({ queryKey: ["warehouse", "transfers"] });
  };

  const addLines = useMutation({
    mutationFn: (barcodes: string[]) =>
      warehouseService.addTransferLines(uuid, barcodes),
    onSuccess: (next) => {
      setLineError(null);
      apply(next);
    },
    onError: (err) =>
      setLineError(
        warehouseErrorMessage(err, t, t("warehouse.entry.add_failed")),
      ),
  });
  const removeLine = useMutation({
    mutationFn: (lineUuid: string) =>
      warehouseService.deleteTransferLine(uuid, lineUuid),
    onSuccess: apply,
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const place = useMutation({
    mutationFn: (
      target: { location_uuid: string } | { location_code: string },
    ) =>
      warehouseService.placeTransfer(uuid, {
        ...target,
        ...(transfer
          ? { line_uuids: untargetedLines(transfer).map((l) => l.uuid) }
          : {}),
      }),
    onSuccess: (next) => {
      setPlaceError(null);
      apply(next);
      appToast.success(t("warehouse.entry.placed"));
    },
    onError: (err) =>
      setPlaceError(
        warehouseErrorMessage(err, t, t("warehouse.entry.place_failed")),
      ),
  });
  const finish = useMutation({
    mutationFn: (action: Finish) =>
      action === "ship"
        ? warehouseService.shipTransfer(uuid)
        : action === "complete"
          ? warehouseService.completeTransfer(uuid)
          : warehouseService.cancelTransfer(uuid),
    onSuccess: (next, action) => {
      apply(next);
      appToast.success(t(`warehouse.transfer.${action}_done`));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const onUnit = (code: string) => {
    if (scanKind(code) === "location") {
      setLineError(t("warehouse.entry.scan_location_here"));
      return;
    }
    addLines.mutate([unitBarcode(code)]);
  };
  const onLocationScan = (code: string) => {
    if (scanKind(code) === "unit") {
      setPlaceError(t("warehouse.entry.scan_unit_here"));
      return;
    }
    place.mutate({ location_code: code });
  };

  const askFinish = async (action: Finish) => {
    const ok = await confirm({
      title: t(`warehouse.transfer.${action}_title`),
      description: t(`warehouse.transfer.${action}_description`),
      confirmLabel: t(`warehouse.transfer.${action}`),
      variant: action === "cancel" ? "destructive" : "default",
    });
    if (ok) finish.mutate(action);
  };

  const lines = transfer?.lines ?? [];
  const writable = Boolean(transfer && canWrite);

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={
        transfer
          ? t("warehouse.transfer.title_no", { no: transfer.transfer_no })
          : t("warehouse.transfer.title")
      }
      icon={<ArrowLeftRight className="size-6" />}
      crumbs={[
        {
          label: t("warehouse.transfers.title"),
          href: routes.tenant.warehouse.transfers(slug),
        },
      ]}
      actions={
        transfer && writable ? (
          <div className="flex flex-wrap gap-2">
            {canCancel(transfer) ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => void askFinish("cancel")}
                disabled={finish.isPending}
                data-testid="transfer-cancel"
              >
                <XCircle className="size-4" />
                {t("warehouse.transfer.cancel")}
              </Button>
            ) : null}
            {transfer.status === "draft" ? (
              <Button
                type="button"
                onClick={() => void askFinish("ship")}
                disabled={finish.isPending || !canShip(transfer)}
                title={
                  canShip(transfer)
                    ? undefined
                    : t("warehouse.transfer.ship_blocked")
                }
                data-testid="transfer-ship"
              >
                <Truck className="size-4" />
                {t("warehouse.transfer.ship")}
              </Button>
            ) : null}
            {transfer.status === "in_transit" ? (
              <Button
                type="button"
                onClick={() => void askFinish("complete")}
                disabled={
                  finish.isPending || !canCompleteWithoutLocation(transfer)
                }
                title={
                  canCompleteWithoutLocation(transfer)
                    ? undefined
                    : t("warehouse.transfer.complete_blocked")
                }
                data-testid="transfer-complete"
              >
                <CheckCircle2 className="size-4" />
                {t("warehouse.transfer.complete")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {transferQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void transferQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !transfer ? (
        <p className="text-muted-foreground text-sm">
          {t("warehouse.list.loading")}
        </p>
      ) : (
        <>
          <Card>
            <CardContent className="grid gap-4 pt-6 sm:grid-cols-4">
              <Info label={t("warehouse.fields.from_warehouse")}>
                {transfer.from_warehouse.code} · {transfer.from_warehouse.name}
              </Info>
              <Info label={t("warehouse.fields.to_warehouse")}>
                {transfer.to_warehouse.code} · {transfer.to_warehouse.name}
                {transfer.to_location ? (
                  <div
                    className="text-muted-foreground font-mono text-xs"
                    dir="ltr"
                  >
                    {transfer.to_location.full_code ??
                      transfer.to_location.code}
                  </div>
                ) : null}
              </Info>
              <Info label={t("warehouse.entries.columns.status")}>
                <span
                  data-testid="transfer-status"
                  data-status={transfer.status}
                >
                  <StatusChip
                    label={t(`warehouse.transfer_status.${transfer.status}`)}
                    tone={transferStatusTone(transfer.status)}
                  />
                </span>
              </Info>
              <Info label={t("warehouse.entries.columns.lines")}>
                <span data-testid="transfer-counts">
                  {format.number(transfer.line_count)}
                </span>
              </Info>
              {transfer.note ? (
                <div className="sm:col-span-4">
                  <Info label={t("warehouse.fields.note")}>
                    {transfer.note}
                  </Info>
                </div>
              ) : null}
            </CardContent>
          </Card>

          {writable && canEditLines(transfer) ? (
            <Card>
              <CardHeader>
                <CardTitle>{t("warehouse.entry.add_title")}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <ScanInput
                  id="transfer-barcode"
                  label={t("warehouse.entry.scan_barcode")}
                  hint={t("warehouse.transfer.scan_unit_hint")}
                  busy={addLines.isPending}
                  onScan={onUnit}
                />
                {lineError ? (
                  <p
                    role="alert"
                    className="text-destructive text-sm"
                    data-testid="transfer-line-error"
                  >
                    {lineError}
                  </p>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          {writable && canPlace(transfer) ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <MapPin className="size-4" />
                  {t("warehouse.transfer.target_title")}
                </CardTitle>
                <p className="text-muted-foreground text-sm">
                  {t("warehouse.transfer.target_hint", {
                    count: untargetedLines(transfer).length,
                    warehouse: transfer.to_warehouse.code,
                  })}
                </p>
              </CardHeader>
              <CardContent className="grid gap-6 lg:grid-cols-2">
                <ScanInput
                  id="transfer-location"
                  autoFocus={transfer.status === "in_transit"}
                  label={t("warehouse.entry.scan_location")}
                  hint={t("warehouse.entry.scan_location_hint")}
                  busy={place.isPending}
                  onScan={onLocationScan}
                />
                <div className="space-y-2">
                  <LocationPicker
                    warehouseUuid={transfer.to_warehouse.uuid}
                    value={locationUuid}
                    onChange={setLocationUuid}
                    testId="transfer-pick"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    disabled={!locationUuid || place.isPending}
                    onClick={() =>
                      place.mutate({ location_uuid: locationUuid })
                    }
                    data-testid="transfer-place"
                  >
                    {t("warehouse.entry.place")}
                  </Button>
                </div>
                {placeError ? (
                  <p
                    role="alert"
                    className="text-destructive text-sm lg:col-span-2"
                    data-testid="transfer-place-error"
                  >
                    {placeError}
                  </p>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          <Card>
            <CardHeader>
              <CardTitle>{t("warehouse.entry.lines")}</CardTitle>
            </CardHeader>
            <CardContent>
              {lines.length === 0 ? (
                <p
                  className="text-muted-foreground text-sm"
                  data-testid="transfer-lines-empty"
                >
                  {t("warehouse.entry.no_lines")}
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table
                    className="w-full text-sm"
                    data-testid="transfer-lines"
                  >
                    <thead>
                      <tr className="text-muted-foreground border-b text-xs">
                        <th className="p-2 text-start font-medium">
                          {t("warehouse.fields.barcode")}
                        </th>
                        <th className="p-2 text-start font-medium">
                          {t("warehouse.fields.product")}
                        </th>
                        <th className="p-2 text-start font-medium">
                          {t("warehouse.transfer.source")}
                        </th>
                        <th className="p-2 text-start font-medium">
                          {t("warehouse.transfer.target")}
                        </th>
                        <th className="p-2 text-end font-medium">
                          <span className="sr-only">
                            {t("warehouse.entry.actions")}
                          </span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {lines.map((l) => (
                        <tr
                          key={l.uuid}
                          className="border-b last:border-0"
                          data-testid="transfer-line"
                          data-barcode={l.barcode}
                        >
                          <td className="p-2 font-mono text-xs" dir="ltr">
                            {l.barcode}
                          </td>
                          <td className="p-2">
                            {l.product.name}
                            <div
                              className="text-muted-foreground font-mono text-xs"
                              dir="ltr"
                            >
                              {l.product.sku}
                            </div>
                          </td>
                          <td className="p-2 font-mono text-xs" dir="ltr">
                            {l.source_location?.full_code ??
                              l.source_location?.code ??
                              "—"}
                          </td>
                          <td
                            className="p-2"
                            data-testid="transfer-line-target"
                          >
                            {l.target_location ? (
                              <span className="font-mono text-xs" dir="ltr">
                                {l.target_location.full_code ??
                                  l.target_location.code}
                              </span>
                            ) : (
                              <span className="text-muted-foreground text-xs">
                                {t("warehouse.entry.unplaced")}
                              </span>
                            )}
                          </td>
                          <td className="p-2">
                            {writable && canEditLines(transfer) ? (
                              <div className="flex justify-end">
                                <Button
                                  type="button"
                                  size="icon-sm"
                                  variant="ghost"
                                  aria-label={t("warehouse.entry.remove_line", {
                                    barcode: l.barcode,
                                  })}
                                  onClick={() => removeLine.mutate(l.uuid)}
                                  disabled={removeLine.isPending}
                                >
                                  <Trash2 className="size-4" />
                                </Button>
                              </div>
                            ) : null}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </WarehouseShell>
  );
}
