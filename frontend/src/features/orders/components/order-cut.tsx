"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { AlertTriangle, PackageOpen, Scissors } from "lucide-react";
import { useDeferredValue, useState } from "react";
import { toast } from "@/lib/toast";

import { Badge } from "@/components/ui/badge";
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
import { Textarea } from "@/components/ui/textarea";
import { useStoreOrder } from "@/features/orders/components/order-action-dialog";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  orderKeys,
  ordersService,
  type Order,
  type OrderCutPreview,
  type OrderItem,
} from "@/features/orders/services/orders.service";
import { stockService } from "@/features/stock/services/stock.service";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";

const NOTE_MAX = 500;

/**
 * TEC-1241: a meter line of a box product (sold by the meter, the line
 * carries credits). It is picked by cutting one piece from an open roll
 * or from a closed box opened on confirmation.
 */
export function isCutLine(
  item: Pick<OrderItem, "sale_measure" | "line_credits">,
) {
  return item.sale_measure === "meter" && item.line_credits != null;
}

/** Normalises a typed meter value ("3,5" -> "3.5"); "" when invalid. */
export function cutMeters(raw: string): string {
  const v = raw.trim().replace(",", ".");
  return /^\d{1,8}(\.\d{1,2})?$/.test(v) && Number(v) > 0 ? v : "";
}

/** The warnings the confirmation shows for a preview (keys and values). */
export function cutWarnings(p: OrderCutPreview): {
  key: string;
  values: Record<string, string>;
}[] {
  const out: { key: string; values: Record<string, string> }[] = [];
  if (p.closed_box) {
    out.push({
      key: p.whole ? "orders.cut.box_whole" : "orders.cut.box_open",
      values: {
        cut: p.cut_meters,
        rest: p.remaining_meters,
        box: p.source_meters,
      },
    });
  }
  if (p.remnant) {
    out.push({
      key: "orders.cut.remnant",
      values: { rest: p.remaining_meters, min: p.min_cut_meters ?? "" },
    });
  }
  if (p.partial) {
    out.push({
      key: "orders.cut.partial_short",
      values: { cut: p.cut_meters, missing: p.missing_meters },
    });
  }
  return out;
}

function newKey(): string {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** Downloads the label of the cut piece (printed automatically, K5). */
async function printPieceLabel(barcode: string, failed: string) {
  try {
    const { blob, filename } = await stockService.unitLabels([barcode]);
    triggerBrowserDownload(blob, filename ?? `${barcode}.pdf`);
  } catch {
    toast.error(failed);
  }
}

/**
 * TEC-1241: the full confirmation of a scanned roll or closed box on a
 * meter line. It names what happens ("closed box: 3 m are cut, 12 m stay
 * as an open roll"), warns about a remnant and the credits the piece
 * carries. Enter confirms. "Parçalı gönder" (partial) asks for the piece
 * length, a note and the credits; the default flow cuts one piece.
 */
export function CutDialog({
  order,
  item,
  barcode,
  partial: startPartial = false,
  onOpenChange,
}: {
  order: Order;
  item: OrderItem;
  /** The scanned barcode; null closes the dialog. */
  barcode: string | null;
  partial?: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const [partial, setPartial] = useState(startPartial);
  const [meters, setMeters] = useState("");
  const [note, setNote] = useState("");
  const [credits, setCredits] = useState("");
  const [key, setKey] = useState(newKey);
  // The caller mounts the dialog with key={barcode}: each scan starts
  // with a fresh form and a fresh idempotency key.
  const open = barcode != null;
  const typed = useDeferredValue(partial ? cutMeters(meters) : "");
  const preview = useQuery({
    queryKey: orderKeys.cutPreview(order.uuid, item.uuid, barcode ?? "", typed),
    queryFn: () =>
      ordersService.cutPreview(
        order.uuid,
        item.uuid,
        barcode ?? "",
        typed || undefined,
      ),
    enabled: open,
    staleTime: 0,
    retry: false,
  });
  const p = preview.data;
  const needsPartial = !!p?.partial;
  const creditsValue = credits.trim() === "" ? null : Number(credits);
  const creditsOk =
    creditsValue == null ||
    (Number.isInteger(creditsValue) &&
      creditsValue >= 0 &&
      creditsValue <= 1000);
  const ready =
    !!p &&
    (!needsPartial || (partial && note.trim() !== "")) &&
    creditsOk &&
    (!partial || meters.trim() === "" || cutMeters(meters) !== "");
  const mutation = useMutation({
    mutationFn: () =>
      ordersService.assignUnit(order.uuid, item.uuid, {
        barcode: barcode ?? "",
        idempotency_key: key,
        ...(p?.closed_box ? { open_box: true } : {}),
        ...(partial && cutMeters(meters) ? { meters: cutMeters(meters) } : {}),
        ...(needsPartial ? { partial: true, note: note.trim() } : {}),
        ...(partial && creditsValue != null ? { credits: creditsValue } : {}),
      }),
    onSuccess: (updated) => {
      store(updated);
      const label = updated.split?.new_barcode ?? barcode ?? "";
      toast.success(
        updated.split
          ? t("orders.cut.success", {
              barcode: updated.split.new_barcode,
              rest: updated.split.source_remaining_meters,
            })
          : t("orders.cut.success_whole", { barcode: label }),
      );
      // The label of the cut piece is printed right away.
      if (label) void printPieceLabel(label, t("warehouse.labels.failed"));
      onOpenChange(false);
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.assign.failed"))),
  });
  const submit = () => {
    if (ready && !mutation.isPending) mutation.mutate();
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg" data-testid="cut-dialog">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {p?.closed_box ? (
                <PackageOpen className="size-5" />
              ) : (
                <Scissors className="size-5" />
              )}
              {p?.closed_box
                ? t("orders.cut.title_box")
                : t("orders.cut.title")}
            </DialogTitle>
            <DialogDescription>
              {item.product.name} · <span dir="ltr">{barcode}</span>
              {p?.location_code ? (
                <>
                  {" · "}
                  <span dir="ltr">{p.location_code}</span>
                </>
              ) : null}
            </DialogDescription>
          </DialogHeader>
          {preview.isLoading ? (
            <p className="text-muted-foreground text-sm">
              {t("orders.loading")}
            </p>
          ) : preview.isError ? (
            <p
              role="alert"
              className="text-destructive text-sm"
              data-testid="cut-error"
            >
              {orderErrorMessage(preview.error, t, t("orders.assign.failed"))}
            </p>
          ) : p ? (
            <div className="space-y-3">
              <dl
                className="grid grid-cols-3 gap-2 rounded border p-3 text-center"
                data-testid="cut-summary"
              >
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {p.closed_box ? t("orders.cut.box") : t("orders.cut.roll")}
                  </dt>
                  <dd className="text-lg font-semibold tabular-nums" dir="ltr">
                    {p.source_meters} {t("orders.unit.m")}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("orders.cut.cut")}
                  </dt>
                  <dd
                    className="text-primary text-lg font-semibold tabular-nums"
                    dir="ltr"
                  >
                    {p.cut_meters} {t("orders.unit.m")}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("orders.cut.rest")}
                  </dt>
                  <dd className="text-lg font-semibold tabular-nums" dir="ltr">
                    {p.remaining_meters} {t("orders.unit.m")}
                  </dd>
                </div>
              </dl>
              {cutWarnings(p).map((w) => (
                <p
                  key={w.key}
                  className="flex items-start gap-2 rounded bg-amber-50 p-2 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100"
                  data-testid="cut-warning"
                >
                  <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                  {t(w.key, w.values)}
                </p>
              ))}
              <p className="text-sm" data-testid="cut-credits">
                {t("orders.cut.credits", {
                  credits: String(p.credits),
                  line: String(p.line_credits),
                })}
                {p.plotter_cut ? (
                  <Badge variant="secondary" className="ms-2">
                    {t("orders.plotter.badge")}
                  </Badge>
                ) : null}
              </p>
              {needsPartial && !partial ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="cut-switch-partial"
                  onClick={() => setPartial(true)}
                >
                  {t("orders.cut.partial_button")}
                </Button>
              ) : null}
            </div>
          ) : null}
          {partial ? (
            <fieldset
              className="space-y-3 rounded border p-3"
              data-testid="cut-partial"
            >
              <legend className="px-1 text-sm font-medium">
                {t("orders.cut.partial_title")}
              </legend>
              <p className="text-muted-foreground text-xs">
                {t("orders.cut.partial_hint")}
              </p>
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1">
                  <Label htmlFor="cut-meters" className="text-xs">
                    {t("orders.cut.piece_meters")}
                  </Label>
                  <Input
                    id="cut-meters"
                    inputMode="decimal"
                    dir="ltr"
                    value={meters}
                    placeholder={p?.cut_meters}
                    onChange={(e) => {
                      setMeters(e.target.value);
                      setKey(newKey());
                    }}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="cut-credits" className="text-xs">
                    {t("orders.cut.piece_credits")}
                  </Label>
                  <Input
                    id="cut-credits"
                    inputMode="numeric"
                    dir="ltr"
                    value={credits}
                    placeholder={p ? String(p.credits) : ""}
                    aria-invalid={!creditsOk}
                    onChange={(e) => setCredits(e.target.value)}
                  />
                </div>
              </div>
              <div className="space-y-1">
                <Label htmlFor="cut-note" className="text-xs">
                  {t("orders.cut.note")}
                </Label>
                <Textarea
                  id="cut-note"
                  rows={2}
                  maxLength={NOTE_MAX}
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                />
              </div>
            </fieldset>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("orders.actions.back")}
            </Button>
            <Button
              type="submit"
              autoFocus
              disabled={!ready || mutation.isPending}
              data-testid="cut-confirm"
            >
              {p?.closed_box
                ? t("orders.cut.confirm_box")
                : t("orders.cut.confirm")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * TEC-1241: cut pieces of the order that are no longer on their line
 * (cancelled order or unassigned piece); they stay in stock separately.
 */
export function DetachedPiecesNotice({ order }: { order: Order }) {
  const { t } = useLocale();
  const pieces = order.detached_cut_pieces ?? [];
  if (pieces.length === 0) return null;
  return (
    <div
      role="status"
      className="flex items-start gap-2 rounded border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-100"
      data-testid="detached-pieces"
    >
      <AlertTriangle className="mt-0.5 size-4 shrink-0" />
      <div className="space-y-1">
        <p>{t("orders.cut.detached", { count: pieces.length })}</p>
        <ul className="font-mono text-xs" dir="ltr">
          {pieces.map((p) => (
            <li key={p.unit_uuid}>
              {p.barcode} · {p.remaining_meters ?? p.cut_meters}{" "}
              {t("orders.unit.m")}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
