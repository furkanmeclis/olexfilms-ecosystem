"use client";

import { useMutation } from "@tanstack/react-query";
import { ClipboardX } from "lucide-react";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import { unitBarcode } from "@/features/warehouse/lib/scan";
import {
  warehouseService,
  type WarehouseTransfer,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

const NOTE_MAX = 500;

type Resolution = "found" | "returned_to_source" | "closed";

type CompleteBody = {
  missing: { line_uuid: string; note?: string }[];
  surplus: { barcode: string }[];
};

/** TEC-1227: the completion body from the receipt check. */
export function completeBody(
  transfer: WarehouseTransfer,
  arrived: Set<string>,
  notes: Record<string, string>,
  surplus: string[],
): CompleteBody {
  return {
    missing: (transfer.lines ?? [])
      .filter((l) => !arrived.has(l.uuid))
      .map((l) => ({
        line_uuid: l.uuid,
        ...(notes[l.uuid]?.trim() ? { note: notes[l.uuid].trim() } : {}),
      })),
    surplus: surplus.map((barcode) => ({ barcode })),
  };
}

/**
 * TEC-1227: receive a transfer. Every line is ticked as arrived; scanning
 * starts from none and ticks the scanned lines. A scanned barcode that is
 * not on the transfer is recorded as surplus. Unticked lines are missing:
 * they stay in transit under the transfer's discrepancy record.
 */
export function CompleteTransferDialog({
  transfer,
  open,
  onClose,
  onDone,
}: {
  transfer: WarehouseTransfer;
  open: boolean;
  onClose: () => void;
  onDone: (next: WarehouseTransfer) => void;
}) {
  const { t } = useLocale();
  const lines = transfer.lines ?? [];
  const all = () => new Set(lines.map((l) => l.uuid));
  const [arrived, setArrived] = useState<Set<string>>(all);
  const [notes, setNotes] = useState<Record<string, string>>({});
  const [surplus, setSurplus] = useState<string[]>([]);
  const [scanning, setScanning] = useState(false);
  const body = completeBody(transfer, arrived, notes, surplus);
  const mutation = useMutation({
    mutationFn: () =>
      warehouseService.completeTransfer(transfer.uuid, {
        ...(body.missing.length ? { missing: body.missing } : {}),
        ...(body.surplus.length ? { surplus: body.surplus } : {}),
      }),
    onSuccess: (next) => {
      appToast.success(t("warehouse.transfer.complete_done"));
      onDone(next);
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const toggle = (uuid: string, on: boolean) =>
    setArrived((s) => {
      const n = new Set(s);
      if (on) n.add(uuid);
      else n.delete(uuid);
      return n;
    });
  const onScan = (code: string) => {
    const barcode = unitBarcode(code).trim();
    const line = lines.find((l) => l.barcode === barcode);
    if (line) {
      toggle(line.uuid, true);
      return;
    }
    if (!surplus.includes(barcode)) setSurplus((s) => [...s, barcode]);
    appToast.error(t("warehouse.discrepancy.surplus_scanned", { barcode }));
  };
  const noneArrived = arrived.size === 0;
  return (
    <Dialog open={open} onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("warehouse.transfer.complete_title")}</DialogTitle>
          <DialogDescription>
            {t("warehouse.discrepancy.complete_hint")}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant={scanning ? "outline" : "default"}
            data-testid="transfer-accept-all"
            onClick={() => {
              setScanning(false);
              setArrived(all());
              setSurplus([]);
            }}
          >
            {t("warehouse.discrepancy.accept_all")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant={scanning ? "default" : "outline"}
            data-testid="transfer-scan-mode"
            onClick={() => {
              setScanning(true);
              setArrived(new Set());
            }}
          >
            {t("warehouse.discrepancy.scan_mode")}
          </Button>
        </div>
        {scanning ? (
          <ScanInput
            id="transfer-receive-scan"
            expect="unit"
            autoFocus
            label={t("warehouse.entry.scan_barcode")}
            hint={t("warehouse.discrepancy.scan_hint", {
              scanned: arrived.size,
              total: lines.length,
            })}
            onScan={onScan}
          />
        ) : null}
        <ul className="max-h-80 space-y-2 overflow-y-auto">
          {lines.map((l) => {
            const ok = arrived.has(l.uuid);
            return (
              <li
                key={l.uuid}
                className="space-y-1.5 rounded-md border p-2 text-sm"
                data-testid="transfer-receive-line"
              >
                <label className="flex items-center gap-2">
                  <Checkbox
                    checked={ok}
                    onCheckedChange={(v) => toggle(l.uuid, v === true)}
                  />
                  <span className="font-mono" dir="ltr">
                    {l.barcode}
                  </span>
                  <span className="text-muted-foreground truncate">
                    {l.product.name}
                  </span>
                  {!ok ? (
                    <Badge variant="danger" className="ms-auto">
                      {t("warehouse.discrepancy.kind.missing")}
                    </Badge>
                  ) : null}
                </label>
                {!ok ? (
                  <Input
                    value={notes[l.uuid] ?? ""}
                    maxLength={NOTE_MAX}
                    placeholder={t("warehouse.discrepancy.note_placeholder")}
                    onChange={(e) =>
                      setNotes((n) => ({ ...n, [l.uuid]: e.target.value }))
                    }
                  />
                ) : null}
              </li>
            );
          })}
        </ul>
        {surplus.length ? (
          <div className="space-y-1 text-sm" data-testid="transfer-surplus">
            <p className="font-medium">
              {t("warehouse.discrepancy.kind.surplus")}
            </p>
            <p className="font-mono text-xs" dir="ltr">
              {surplus.join(", ")}
            </p>
          </div>
        ) : null}
        {body.missing.length || body.surplus.length ? (
          <p className="text-muted-foreground text-xs">
            {t("warehouse.discrepancy.summary", {
              missing: body.missing.length,
              surplus: body.surplus.length,
            })}
          </p>
        ) : null}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            data-testid="transfer-complete-confirm"
            disabled={mutation.isPending || noneArrived}
            title={
              noneArrived ? t("warehouse.discrepancy.none_arrived") : undefined
            }
            onClick={() => mutation.mutate()}
          >
            {t("warehouse.transfer.complete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * TEC-1227: the discrepancy record (tutanak) of a completed transfer:
 * missing lines are found (placed in the target warehouse) or returned to
 * their source; surplus rows are closed.
 */
export function TransferDiscrepanciesCard({
  transfer,
  writable,
  onChange,
}: {
  transfer: WarehouseTransfer;
  writable: boolean;
  onChange: (next: WarehouseTransfer) => void;
}) {
  const { t, format } = useLocale();
  const [locationUuid, setLocationUuid] = useState("");
  const resolve = useMutation({
    mutationFn: (v: { uuid: string; resolution: Resolution }) =>
      warehouseService.resolveTransferDiscrepancy(transfer.uuid, v.uuid, {
        resolution: v.resolution,
        ...(v.resolution === "found" && locationUuid
          ? { location_uuid: locationUuid }
          : {}),
      }),
    onSuccess: (next) => {
      appToast.success(t("warehouse.discrepancy.resolved"));
      onChange(next);
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const rows = transfer.discrepancies ?? [];
  if (rows.length === 0) return null;
  const openMissing = rows.some(
    (d) => d.status === "open" && d.kind === "missing",
  );
  return (
    <Card data-testid="transfer-discrepancies">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ClipboardX className="size-4" />
          {t("warehouse.discrepancy.card")}
          {transfer.open_discrepancies ? (
            <Badge variant="danger">
              {t("warehouse.discrepancy.open_count", {
                count: format.number(transfer.open_discrepancies),
              })}
            </Badge>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {writable && openMissing ? (
          <div className="max-w-md space-y-1">
            <p className="text-muted-foreground text-xs">
              {t("warehouse.discrepancy.found_location")}
            </p>
            <LocationPicker
              warehouseUuid={transfer.to_warehouse.uuid}
              value={locationUuid}
              onChange={setLocationUuid}
              testId="discrepancy-location"
            />
          </div>
        ) : null}
        <ul className="space-y-2">
          {rows.map((d) => (
            <li
              key={d.uuid}
              className="space-y-1.5 rounded-md border p-2 text-sm"
              data-testid="transfer-discrepancy-row"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="min-w-0 truncate">
                  <span className="font-mono" dir="ltr">
                    {d.barcode}
                  </span>
                  {d.product_name ? ` · ${d.product_name}` : null}
                </span>
                <div className="flex gap-1">
                  <Badge variant="outline">
                    {t(`warehouse.discrepancy.kind.${d.kind}`)}
                  </Badge>
                  <StatusChip
                    label={
                      d.resolution
                        ? t(`warehouse.discrepancy.resolution.${d.resolution}`)
                        : t("warehouse.discrepancy.status_open")
                    }
                    tone={d.status === "open" ? "warning" : "success"}
                  />
                </div>
              </div>
              {d.note ? (
                <p className="text-muted-foreground">{d.note}</p>
              ) : null}
              {writable && d.status === "open" ? (
                <div className="flex flex-wrap gap-2">
                  {(d.kind === "missing"
                    ? (["found", "returned_to_source"] as const)
                    : (["closed"] as const)
                  ).map((r) => (
                    <Button
                      key={r}
                      type="button"
                      size="sm"
                      variant={r === "found" ? "default" : "outline"}
                      data-testid={`transfer-discrepancy-${r}`}
                      disabled={resolve.isPending}
                      onClick={() =>
                        resolve.mutate({ uuid: d.uuid, resolution: r })
                      }
                    >
                      {t(`warehouse.discrepancy.resolve.${r}`)}
                    </Button>
                  ))}
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
