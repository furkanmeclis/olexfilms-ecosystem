"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Download,
  ExternalLink,
  FileUp,
  MapPin,
  RefreshCw,
  Sparkles,
  Truck,
} from "lucide-react";
import { useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";

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
import { Label } from "@/components/ui/label";
import {
  RECEIPT_ACCEPT,
  RECEIPT_MAX_BYTES,
  useStoreOrder,
} from "@/features/orders/components/order-action-dialog";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import { isRoll } from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type Order,
  type OrderDocumentKind,
  type OrderItem,
  type OrderSuggestion,
} from "@/features/orders/services/orders.service";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";

/** Amount of a suggestion: meters for rolls, else the quantity. */
export function suggestionAmount(
  s: Pick<OrderSuggestion, "meters" | "quantity">,
  t: (key: string) => string,
): string {
  if (s.meters) return `${s.meters} ${t("orders.unit.m")}`;
  return `${s.quantity ?? 1} ${t("orders.unit.pcs")}`;
}

/**
 * "Suggest and fill" (TEC-768): the server proposes the seller's units for
 * the rest of the line (placed first, oldest entry first; units without a
 * location on request). Nothing is reserved until the user confirms.
 */
export function SuggestDialog({
  order,
  item,
  open,
  onOpenChange,
}: {
  order: Order;
  item: OrderItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const [unlocated, setUnlocated] = useState(false);
  const roll = isRoll(item.product.unit_type);
  const query = useQuery({
    queryKey: orderKeys.suggestions(order.uuid, item.uuid, unlocated),
    queryFn: () => ordersService.suggest(order.uuid, item.uuid, unlocated),
    enabled: open,
    staleTime: 0,
  });
  const rows = query.data?.suggestions ?? [];
  const apply = useMutation({
    mutationFn: () =>
      ordersService.applySuggestions(order.uuid, item.uuid, rows),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.suggest.applied", { count: rows.length }));
      onOpenChange(false);
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.suggest.failed"))),
  });
  const unit = roll ? t("orders.unit.m") : t("orders.unit.pcs");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl" data-testid="suggest-dialog">
        <DialogHeader>
          <DialogTitle>{t("orders.suggest.title")}</DialogTitle>
          <DialogDescription>
            {item.product.name} · <span dir="ltr">{item.product.sku}</span>
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <Checkbox
            id={`unlocated-${item.uuid}`}
            checked={unlocated}
            onCheckedChange={(v) => setUnlocated(v === true)}
            data-testid="suggest-unlocated"
          />
          <Label htmlFor={`unlocated-${item.uuid}`} className="text-sm">
            {t("orders.suggest.include_unlocated")}
          </Label>
        </div>
        <p className="text-muted-foreground text-xs">
          {t("orders.suggest.rule")}
        </p>
        {query.isLoading ? (
          <p className="text-muted-foreground text-sm">{t("orders.loading")}</p>
        ) : query.isError ? (
          <p className="text-destructive text-sm">
            {orderErrorMessage(query.error, t, t("orders.suggest.failed"))}
          </p>
        ) : rows.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="suggest-empty"
          >
            {t("orders.suggest.empty")}
          </p>
        ) : (
          <ul className="max-h-80 divide-y overflow-y-auto rounded border text-sm">
            {rows.map((s) => (
              <li
                key={s.unit_uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                data-testid="suggestion"
              >
                <span className="font-mono text-xs" dir="ltr">
                  {s.barcode}
                </span>
                <span className="text-muted-foreground flex items-center gap-1 text-xs">
                  <MapPin className="size-3" />
                  {s.location_code ? (
                    <span dir="ltr">{s.location_code}</span>
                  ) : (
                    t("orders.picking.no_location")
                  )}
                </span>
                <span className="tabular-nums">{suggestionAmount(s, t)}</span>
              </li>
            ))}
          </ul>
        )}
        {query.data ? (
          <p className="text-sm" data-testid="suggest-coverage">
            {t("orders.suggest.coverage", {
              covered: query.data.covered,
              missing: query.data.missing,
              unit,
            })}
            {!query.data.complete ? (
              <Badge variant="warning" className="ms-2">
                {t("orders.suggest.incomplete")}
              </Badge>
            ) : null}
          </p>
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
            type="button"
            data-testid="suggest-apply"
            disabled={rows.length === 0 || apply.isPending}
            onClick={() => apply.mutate()}
          >
            {t("orders.suggest.apply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Where the seller keeps the line's product (location code, pcs, m). */
export function PickLocationsDialog({
  order,
  item,
  open,
  onOpenChange,
}: {
  order: Order;
  item: OrderItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const roll = isRoll(item.product.unit_type);
  const query = useQuery({
    queryKey: orderKeys.pickLocations(order.uuid, item.uuid),
    queryFn: () => ordersService.pickLocations(order.uuid, item.uuid),
    enabled: open,
  });
  const rows = query.data ?? [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="pick-locations-dialog">
        <DialogHeader>
          <DialogTitle>{t("orders.picking.locations_title")}</DialogTitle>
          <DialogDescription>
            {item.product.name} · <span dir="ltr">{item.product.sku}</span>
          </DialogDescription>
        </DialogHeader>
        {query.isLoading ? (
          <p className="text-muted-foreground text-sm">{t("orders.loading")}</p>
        ) : rows.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("orders.picking.locations_empty")}
          </p>
        ) : (
          <ul className="divide-y rounded border text-sm">
            {rows.map((l, i) => (
              <li
                key={l.location_uuid ?? `none-${i}`}
                className="flex items-center justify-between gap-2 px-3 py-2"
                data-testid="pick-location"
              >
                <span className="flex items-center gap-1">
                  <MapPin className="text-muted-foreground size-3" />
                  {l.location_code ? (
                    <span className="font-mono text-xs" dir="ltr">
                      {l.location_code}
                    </span>
                  ) : (
                    <span className="text-muted-foreground">
                      {t("orders.picking.no_location")}
                    </span>
                  )}
                </span>
                <span className="tabular-nums">
                  {l.quantity} {t("orders.unit.pcs")}
                  {roll && l.meters
                    ? ` · ${l.meters} ${t("orders.unit.m")}`
                    : ""}
                </span>
              </li>
            ))}
          </ul>
        )}
      </DialogContent>
    </Dialog>
  );
}

/** Line buttons while preparing: suggest and fill, pick locations. */
export function LinePickingActions({
  order,
  item,
  suggest,
}: {
  order: Order;
  item: OrderItem;
  suggest: boolean;
}) {
  const { t } = useLocale();
  const [suggestOpen, setSuggestOpen] = useState(false);
  const [locationsOpen, setLocationsOpen] = useState(false);
  return (
    <div className="flex flex-wrap gap-2">
      {suggest ? (
        <Button
          type="button"
          size="sm"
          variant="secondary"
          data-testid="suggest-open"
          onClick={() => setSuggestOpen(true)}
        >
          <Sparkles className="size-4" />
          {t("orders.suggest.button")}
        </Button>
      ) : null}
      <Button
        type="button"
        size="sm"
        variant="ghost"
        data-testid="pick-locations-open"
        onClick={() => setLocationsOpen(true)}
      >
        <MapPin className="size-4" />
        {t("orders.picking.locations")}
      </Button>
      {suggest ? (
        <SuggestDialog
          order={order}
          item={item}
          open={suggestOpen}
          onOpenChange={setSuggestOpen}
        />
      ) : null}
      <PickLocationsDialog
        order={order}
        item={item}
        open={locationsOpen}
        onOpenChange={setLocationsOpen}
      />
    </div>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  );
}

/** Statuses after the shipment (documents and tracking apply). */
const SHIPPED = new Set(["shipped", "received", "cancelling"]);

/**
 * Shipping card (TEC-768): delivery method, tracking number with its link,
 * the shipping note PDF (download; the seller regenerates it) and the
 * buyer's delivery receipt (download; the buyer attaches it).
 */
export function ShippingCard({
  order,
  canShip,
  canReceive,
}: {
  order: Order;
  canShip: boolean;
  canReceive: boolean;
}) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const fileRef = useRef<HTMLInputElement>(null);
  const seller = order.role === "seller" && canShip;
  const buyer =
    order.role === "buyer" &&
    canReceive &&
    (order.status === "shipped" || order.status === "received");
  const download = useMutation({
    mutationFn: async (kind: OrderDocumentKind) => {
      const { blob, filename } = await ordersService.downloadDocument(
        order.uuid,
        kind,
      );
      triggerBrowserDownload(
        blob,
        filename ??
          `${kind}-${order.order_no}.${kind === "shipping" ? "pdf" : "bin"}`,
      );
    },
    onError: () => toast.error(t("orders.shipping.download_failed")),
  });
  const regenerate = useMutation({
    mutationFn: () => ordersService.regenerateShippingDocument(order.uuid),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.shipping.regenerated"));
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("orders.shipping.regenerate_failed")),
      ),
  });
  const upload = useMutation({
    mutationFn: (file: File) => ordersService.attachReceipt(order.uuid, file),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.shipping.receipt_uploaded"));
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("orders.shipping.receipt_failed")),
      ),
  });
  if (!SHIPPED.has(order.status) && !order.delivery_method) return null;
  return (
    <Card data-testid="order-shipping">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Truck className="text-muted-foreground size-4" />
          {t("orders.shipping.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid gap-3 sm:grid-cols-2">
          <Row label={t("orders.shipping.method")}>
            <span data-testid="shipping-method">
              {order.delivery_method?.name ?? "—"}
            </span>
          </Row>
          <Row label={t("orders.shipping.tracking_no")}>
            {order.tracking_no ? (
              order.tracking_url ? (
                <a
                  href={order.tracking_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary inline-flex items-center gap-1 underline"
                  dir="ltr"
                  data-testid="tracking-link"
                >
                  {order.tracking_no}
                  <ExternalLink className="size-3" />
                </a>
              ) : (
                <span dir="ltr" data-testid="tracking-no">
                  {order.tracking_no}
                </span>
              )
            ) : (
              "—"
            )}
          </Row>
        </dl>
        <div className="flex flex-wrap gap-2">
          {order.has_shipping_document ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              data-testid="shipping-document-download"
              disabled={download.isPending}
              onClick={() => download.mutate("shipping")}
            >
              <Download className="size-4" />
              {t("orders.shipping.download_note")}
            </Button>
          ) : null}
          {seller && SHIPPED.has(order.status) ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              data-testid="shipping-document-regenerate"
              disabled={regenerate.isPending}
              onClick={() => regenerate.mutate()}
            >
              <RefreshCw className="size-4" />
              {order.has_shipping_document
                ? t("orders.shipping.regenerate")
                : t("orders.shipping.generate")}
            </Button>
          ) : null}
          {order.has_receipt_document ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              data-testid="receipt-download"
              disabled={download.isPending}
              onClick={() => download.mutate("receipt")}
            >
              <Download className="size-4" />
              {t("orders.shipping.download_receipt")}
            </Button>
          ) : null}
          {buyer ? (
            <>
              <input
                ref={fileRef}
                type="file"
                accept={RECEIPT_ACCEPT}
                className="hidden"
                data-testid="receipt-input"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (!file) return;
                  if (file.size > RECEIPT_MAX_BYTES) {
                    toast.error(t("orders.shipping.receipt_too_large"));
                    return;
                  }
                  upload.mutate(file);
                }}
              />
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={upload.isPending}
                onClick={() => fileRef.current?.click()}
              >
                <FileUp className="size-4" />
                {order.has_receipt_document
                  ? t("orders.shipping.replace_receipt")
                  : t("orders.shipping.attach_receipt")}
              </Button>
            </>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
