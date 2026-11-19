"use client";

import { useMutation } from "@tanstack/react-query";
import {
  AlertTriangle,
  Camera,
  Image as ImageIcon,
  Link2,
  PackageMinus,
} from "lucide-react";
import Link from "next/link";
import { useRef, useState } from "react";
import { toast } from "@/lib/toast";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
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
import { routes } from "@/config/routes";
import { useStoreOrder } from "@/features/orders/components/order-action-dialog";
import { lineAmount } from "@/features/orders/lib/access";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import { orderStatusTone } from "@/features/orders/lib/form";
import {
  ordersService,
  type Order,
  type OrderDiscrepancyInput,
} from "@/features/orders/services/orders.service";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";

const REASON_MAX = 500;
const PHOTO_ACCEPT = "image/jpeg,image/png,image/webp";
const PHOTO_MAX_BYTES = 10 * 1024 * 1024;

/** A line that is short of its amount: what goes to the backorder. */
export type ShortLine = {
  name: string;
  picked: number;
  rest: number;
  /** TEC-1241: the meter rest is shorter than the minimum cut. */
  belowMinCut?: string;
};

/** TEC-1227: the lines a partial shipment would move to the backorder. */
export function shortLines(order: Order): ShortLine[] {
  return (order.items ?? [])
    .map((it) => {
      const want = lineAmount(it);
      const picked = Number(it.assigned) || 0;
      const rest = want - picked;
      const min = it.min_cut_meters ? Number(it.min_cut_meters) : 0;
      const below =
        it.sale_measure === "meter" &&
        min > 0 &&
        rest > 1e-9 &&
        rest < min - 1e-9;
      return {
        name: it.product.name,
        picked,
        rest,
        ...(below ? { belowMinCut: it.min_cut_meters ?? "" } : {}),
      };
    })
    .filter((l) => l.rest > 1e-9);
}

/**
 * TEC-1227: "Ship partially" on a preparing order with stock short. The
 * picked part stays, the rest becomes a backorder; on success the order is
 * ready and onReady opens the usual ship dialog.
 */
export function PartialShipButton({
  order,
  onReady,
}: {
  order: Order;
  onReady: (updated: Order) => void;
}) {
  const { t, format } = useLocale();
  const store = useStoreOrder();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const short = shortLines(order);
  const anyPicked = (order.items ?? []).some((it) => Number(it.assigned) > 0);
  const mutation = useMutation({
    mutationFn: () =>
      ordersService.partialShip(order.uuid, reason.trim() || undefined),
    onSuccess: (updated) => {
      store(updated);
      toast.success(
        t("orders.partial.success", {
          order_no: updated.backorders?.[0]?.order_no ?? "",
        }),
      );
      setOpen(false);
      setReason("");
      onReady(updated);
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.partial.failed"))),
  });
  if (!order.can_partial_ship || short.length === 0) return null;
  return (
    <>
      <Button
        type="button"
        variant="outline"
        data-testid="order-partial-ship"
        disabled={!anyPicked}
        title={anyPicked ? undefined : t("orders.partial.nothing_picked")}
        onClick={() => setOpen(true)}
      >
        <PackageMinus className="size-4" />
        {t("orders.partial.button")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("orders.partial.title")}</DialogTitle>
            <DialogDescription>
              {t("orders.partial.description")}
            </DialogDescription>
          </DialogHeader>
          <ul className="space-y-1 text-sm" data-testid="partial-short-lines">
            {short.map((l) => (
              <li key={l.name} className="space-y-0.5">
                <div className="flex justify-between gap-2">
                  <span className="truncate">{l.name}</span>
                  <span className="text-muted-foreground shrink-0">
                    {t("orders.partial.line", {
                      picked: format.number(l.picked),
                      rest: format.number(l.rest),
                    })}
                  </span>
                </div>
                {l.belowMinCut ? (
                  <p
                    className="flex items-center gap-1 text-xs text-amber-700 dark:text-amber-300"
                    data-testid="partial-below-min-cut"
                  >
                    <AlertTriangle className="size-3 shrink-0" />
                    {t("orders.cut.partial_ship_below_min", {
                      rest: format.number(l.rest),
                      min: l.belowMinCut,
                    })}
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
          <div className="space-y-1.5">
            <Label htmlFor="partial-reason">
              {t("orders.actions.reason_optional")}
            </Label>
            <Textarea
              id="partial-reason"
              rows={2}
              maxLength={REASON_MAX}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
            >
              {t("orders.actions.back")}
            </Button>
            <Button
              type="button"
              data-testid="partial-confirm"
              disabled={mutation.isPending}
              onClick={() => mutation.mutate()}
            >
              {t("orders.partial.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

/** TEC-1227: the list / header mark of a backorder. */
export function BackorderBadge({
  order,
}: {
  order: Pick<Order, "is_backorder">;
}) {
  const { t } = useLocale();
  if (!order.is_backorder) return null;
  return (
    <Badge variant="outline" data-testid="order-backorder-badge">
      {t("orders.partial.badge")}
    </Badge>
  );
}

/** TEC-1227: the order a backorder was split from and its backorders. */
export function OrderBackordersCard({
  slug,
  order,
}: {
  slug: string;
  order: Order;
}) {
  const { t } = useLocale();
  const source = order.backorder_of;
  const rows = order.backorders ?? [];
  if (!source && rows.length === 0) return null;
  const groups = [
    ...(source
      ? [{ title: t("orders.partial.source"), id: "source", links: [source] }]
      : []),
    ...(rows.length
      ? [{ title: t("orders.partial.list"), id: "backorder", links: rows }]
      : []),
  ];
  return (
    <Card data-testid="order-backorders">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Link2 className="size-4" />
          {t("orders.partial.card")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {groups.map((g) => (
          <div key={g.id} className="space-y-1.5">
            <p className="text-muted-foreground text-xs">{g.title}</p>
            <ul className="space-y-2">
              {g.links.map((o) => (
                <li
                  key={o.uuid}
                  className="flex items-center justify-between gap-2 text-sm"
                >
                  <Link
                    href={routes.tenant.orders.detail(slug, o.uuid)}
                    className="font-mono font-medium hover:underline"
                    dir="ltr"
                    data-testid={`order-${g.id}-link`}
                  >
                    {o.order_no}
                  </Link>
                  <StatusChip
                    label={o.status_label}
                    tone={orderStatusTone(o.status)}
                  />
                </li>
              ))}
            </ul>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

/** A shipped unit of the receipt form. */
type ShippedUnit = {
  unitUuid: string;
  barcode: string;
  product: string;
};

function shippedUnits(order: Order): ShippedUnit[] {
  return (order.items ?? []).flatMap((it) =>
    (it.units ?? []).map((u) => ({
      unitUuid: u.unit_uuid,
      barcode: u.barcode,
      product: it.product.name,
    })),
  );
}

/** Receipt state of one unit: received, missing or damaged with a reason. */
export type ReceiptMark = {
  kind: "ok" | "missing" | "damaged";
  reason: string;
};

/** The discrepancies to send with the received step. */
export function receiptDiscrepancies(
  marks: Record<string, ReceiptMark>,
): OrderDiscrepancyInput[] {
  return Object.entries(marks)
    .filter(([, m]) => m.kind !== "ok")
    .map(([unit, m]) => ({
      unit_uuid: unit,
      kind: m.kind as "missing" | "damaged",
      reason: m.reason.trim(),
    }));
}

/** Every reported unit has a reason and at least one unit is received. */
export function receiptMarksValid(
  order: Order,
  marks: Record<string, ReceiptMark>,
): boolean {
  const units = shippedUnits(order);
  const list = receiptDiscrepancies(marks);
  const missing = list.filter((d) => d.kind === "missing").length;
  return (
    list.every((d) => d.reason.length > 0) &&
    (units.length === 0 || missing < units.length)
  );
}

/**
 * TEC-1227: the receipt check of the received step. "Accept all" keeps
 * every unit; scanning marks the scanned units received and the others
 * missing; each unit can be set missing or damaged with a reason.
 */
export function ReceiptCheckFields({
  order,
  marks,
  onChange,
}: {
  order: Order;
  marks: Record<string, ReceiptMark>;
  onChange: (next: Record<string, ReceiptMark>) => void;
}) {
  const { t } = useLocale();
  const units = shippedUnits(order);
  const [scanning, setScanning] = useState(false);
  const [scanned, setScanned] = useState<Set<string>>(new Set());
  if (units.length === 0) return null;
  const mark = (u: string): ReceiptMark =>
    marks[u] ?? { kind: "ok", reason: "" };
  const set = (u: string, m: ReceiptMark) => onChange({ ...marks, [u]: m });
  const startScan = () => {
    setScanning(true);
    setScanned(new Set());
    const next: Record<string, ReceiptMark> = {};
    for (const u of units) {
      next[u.unitUuid] = {
        kind: "missing",
        reason: t("orders.receipt.not_scanned"),
      };
    }
    onChange(next);
  };
  const onScan = (code: string) => {
    const u = units.find((x) => x.barcode === code.trim());
    if (!u) {
      toast.error(t("orders.receipt.not_on_order"));
      return;
    }
    setScanned((s) => new Set(s).add(u.unitUuid));
    set(u.unitUuid, { kind: "ok", reason: "" });
  };
  return (
    <div className="space-y-3" data-testid="receipt-check">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          variant={scanning ? "outline" : "default"}
          data-testid="receipt-accept-all"
          onClick={() => {
            setScanning(false);
            onChange({});
          }}
        >
          {t("orders.receipt.accept_all")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant={scanning ? "default" : "outline"}
          data-testid="receipt-scan-mode"
          onClick={startScan}
        >
          {t("orders.receipt.scan_mode")}
        </Button>
      </div>
      {scanning ? (
        <ScanInput
          id="receipt-scan"
          expect="unit"
          onScan={onScan}
          label={t("orders.receipt.scan_label")}
          hint={t("orders.receipt.scan_hint", {
            scanned: scanned.size,
            total: units.length,
          })}
          autoFocus
        />
      ) : null}
      <ul className="max-h-72 space-y-2 overflow-y-auto">
        {units.map((u) => {
          const m = mark(u.unitUuid);
          return (
            <li
              key={u.unitUuid}
              className="space-y-1.5 rounded-md border p-2"
              data-testid="receipt-unit"
            >
              <div className="flex items-center justify-between gap-2 text-sm">
                <span className="min-w-0 truncate">
                  <span className="font-mono" dir="ltr">
                    {u.barcode}
                  </span>{" "}
                  · {u.product}
                </span>
                <Select
                  value={m.kind}
                  onValueChange={(v) =>
                    set(u.unitUuid, {
                      kind: v as ReceiptMark["kind"],
                      reason: v === "ok" ? "" : m.reason,
                    })
                  }
                >
                  <SelectTrigger
                    className="w-36 shrink-0"
                    data-testid="receipt-unit-kind"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ok">
                      {t("orders.receipt.kind.ok")}
                    </SelectItem>
                    <SelectItem value="missing">
                      {t("orders.receipt.kind.missing")}
                    </SelectItem>
                    <SelectItem value="damaged">
                      {t("orders.receipt.kind.damaged")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {m.kind !== "ok" ? (
                <Input
                  value={m.reason}
                  maxLength={REASON_MAX}
                  placeholder={t("orders.receipt.reason_placeholder")}
                  data-testid="receipt-unit-reason"
                  onChange={(e) =>
                    set(u.unitUuid, { ...m, reason: e.target.value })
                  }
                />
              ) : null}
            </li>
          );
        })}
      </ul>
      <p className="text-muted-foreground text-xs">
        {t("orders.receipt.hint")}
      </p>
    </div>
  );
}

/**
 * TEC-1227: the receipt records of an order (missing / damaged units),
 * their resolutions and photos. No price is shown.
 */
export function OrderDiscrepanciesCard({ order }: { order: Order }) {
  const { t, format } = useLocale();
  const store = useStoreOrder();
  const fileRef = useRef<HTMLInputElement>(null);
  const [photoFor, setPhotoFor] = useState<string | null>(null);
  const resolve = useMutation({
    mutationFn: (v: { uuid: string; resolution: string }) =>
      ordersService.resolveDiscrepancy(order.uuid, v.uuid, v.resolution),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.receipt.resolved"));
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("orders.receipt.resolve_failed")),
      ),
  });
  const photo = useMutation({
    mutationFn: (v: { uuid: string; file: File }) =>
      ordersService.addDiscrepancyPhoto(order.uuid, v.uuid, v.file),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.receipt.photo_added"));
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.receipt.photo_failed"))),
  });
  const openPhoto = async (uuid: string, index: number) => {
    try {
      const { blob, filename } = await ordersService.downloadDiscrepancyPhoto(
        order.uuid,
        uuid,
        index,
      );
      triggerBrowserDownload(blob, filename ?? `photo-${index + 1}`);
    } catch (err) {
      toast.error(
        orderErrorMessage(err, t, t("orders.shipping.download_failed")),
      );
    }
  };
  const rows = order.discrepancies ?? [];
  if (rows.length === 0) return null;
  return (
    <Card data-testid="order-discrepancies">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <AlertTriangle className="size-4" />
          {t("orders.receipt.card")}
          {order.open_discrepancies ? (
            <Badge variant="danger" data-testid="discrepancies-open">
              {t("orders.receipt.open_count", {
                count: format.number(order.open_discrepancies),
              })}
            </Badge>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <input
          ref={fileRef}
          type="file"
          accept={PHOTO_ACCEPT}
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (!file || !photoFor) return;
            if (file.size > PHOTO_MAX_BYTES) {
              toast.error(t("orders.shipping.receipt_too_large"));
              return;
            }
            photo.mutate({ uuid: photoFor, file });
          }}
        />
        <ul className="space-y-3">
          {rows.map((d) => (
            <li
              key={d.uuid}
              className="space-y-1.5 rounded-md border p-2 text-sm"
              data-testid="discrepancy-row"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="min-w-0 truncate">
                  <span className="font-mono" dir="ltr">
                    {d.barcode}
                  </span>{" "}
                  · {d.product_name}
                </span>
                <div className="flex gap-1">
                  <Badge variant="outline">
                    {t(`orders.receipt.kind.${d.kind}`)}
                  </Badge>
                  <StatusChip
                    label={
                      d.resolution
                        ? t(`orders.receipt.resolution.${d.resolution}`)
                        : t("orders.receipt.status_open")
                    }
                    tone={d.status === "open" ? "warning" : "success"}
                  />
                </div>
              </div>
              <p className="text-muted-foreground">{d.reason}</p>
              {d.resolution_note ? (
                <p className="text-muted-foreground text-xs">
                  {d.resolution_note}
                </p>
              ) : null}
              <div className="flex flex-wrap gap-2">
                {Array.from({ length: d.photo_count }, (_, i) => (
                  <Button
                    key={i}
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => void openPhoto(d.uuid, i)}
                  >
                    <ImageIcon className="size-4" />
                    {t("orders.receipt.photo", { n: i + 1 })}
                  </Button>
                ))}
                {d.can_add_photo ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    data-testid="discrepancy-add-photo"
                    disabled={photo.isPending}
                    onClick={() => {
                      setPhotoFor(d.uuid);
                      fileRef.current?.click();
                    }}
                  >
                    <Camera className="size-4" />
                    {t("orders.receipt.add_photo")}
                  </Button>
                ) : null}
                {d.can_resolve_as.map((r) => (
                  <Button
                    key={r}
                    type="button"
                    size="sm"
                    data-testid={`discrepancy-resolve-${r}`}
                    disabled={resolve.isPending}
                    onClick={() =>
                      resolve.mutate({ uuid: d.uuid, resolution: r })
                    }
                  >
                    {t(`orders.receipt.resolve.${r}`)}
                  </Button>
                ))}
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
