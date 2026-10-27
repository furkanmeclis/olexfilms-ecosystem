"use client";

import { useMutation } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
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
  ordersService,
  type Order,
  type OrderItem,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";

const INVOICE_NO_MAX = 64;
const NOTE_MAX = 1000;

function today(): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/**
 * TEC-1030: the seller's accounting records the invoice it cut for a
 * received order (number, date, note) on the lines not invoiced yet; every
 * remaining line is preselected.
 */
export function OrderInvoiceDialog({
  order,
  open,
  onClose,
}: {
  order: Order;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const remaining = useMemo(
    () => (order.items ?? []).filter((it) => !it.invoiced),
    [order.items],
  );
  const [invoiceNo, setInvoiceNo] = useState("");
  const [invoiceDate, setInvoiceDate] = useState(today);
  const [note, setNote] = useState("");
  const [excluded, setExcluded] = useState<Set<string>>(() => new Set());
  const selected = remaining.filter((it) => !excluded.has(it.uuid));

  const mark = useMutation({
    mutationFn: () =>
      ordersService.markInvoiced(order.uuid, {
        invoice_no: invoiceNo.trim(),
        invoice_date: invoiceDate,
        note: note.trim() ? note.trim() : null,
        item_uuids: selected.map((it) => it.uuid),
      }),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.invoice.marked"));
      setInvoiceNo("");
      setNote("");
      setExcluded(new Set());
      onClose();
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.invoice.mark_failed"))),
  });

  const toggle = (item: OrderItem, checked: boolean) =>
    setExcluded((prev) => {
      const next = new Set(prev);
      if (checked) next.delete(item.uuid);
      else next.add(item.uuid);
      return next;
    });
  const valid =
    invoiceNo.trim().length > 0 &&
    invoiceNo.trim().length <= INVOICE_NO_MAX &&
    /^\d{4}-\d{2}-\d{2}$/.test(invoiceDate) &&
    selected.length > 0;

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogContent data-testid="order-invoice-dialog">
        <DialogHeader>
          <DialogTitle>{t("orders.invoice.mark_title")}</DialogTitle>
          <DialogDescription>
            {t("orders.invoice.mark_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="invoice-no">
                {t("orders.invoice.invoice_no")}
              </Label>
              <Input
                id="invoice-no"
                value={invoiceNo}
                maxLength={INVOICE_NO_MAX}
                dir="ltr"
                autoFocus
                onChange={(e) => setInvoiceNo(e.target.value)}
                data-testid="invoice-no"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="invoice-date">
                {t("orders.invoice.invoice_date")}
              </Label>
              <DatePicker
                id="invoice-date"
                value={invoiceDate}
                onChange={setInvoiceDate}
                data-testid="invoice-date"
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="invoice-note">{t("orders.invoice.note")}</Label>
            <Textarea
              id="invoice-note"
              value={note}
              maxLength={NOTE_MAX}
              rows={2}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>{t("orders.invoice.lines")}</Label>
            <ul className="max-h-60 space-y-2 overflow-y-auto rounded-md border p-2">
              {remaining.map((it) => {
                const checked = !excluded.has(it.uuid);
                return (
                  <li key={it.uuid} className="flex items-start gap-2 text-sm">
                    <Checkbox
                      id={`inv-${it.uuid}`}
                      checked={checked}
                      onCheckedChange={(v) => toggle(it, v === true)}
                      data-testid="invoice-line"
                    />
                    <label
                      htmlFor={`inv-${it.uuid}`}
                      className="min-w-0 flex-1"
                    >
                      <span className="font-medium">{it.product.name}</span>
                      <span
                        className="text-muted-foreground ms-2 text-xs"
                        dir="ltr"
                      >
                        {it.product.sku}
                      </span>
                      <span className="text-muted-foreground ms-2 text-xs">
                        {it.meters
                          ? `${it.meters} ${t("orders.unit.m")}`
                          : `${it.quantity ?? 0} ${t("orders.unit.pcs")}`}
                      </span>
                    </label>
                  </li>
                );
              })}
            </ul>
            <p className="text-muted-foreground text-xs">
              {t("orders.invoice.lines_hint", { count: selected.length })}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!valid || mark.isPending}
            onClick={() => mark.mutate()}
            data-testid="invoice-submit"
          >
            {t("orders.invoice.mark")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
