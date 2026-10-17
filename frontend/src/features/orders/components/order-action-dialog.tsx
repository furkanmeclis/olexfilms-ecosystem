"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { OrderAction } from "@/features/orders/lib/access";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  deliveryMethodsService,
  orderKeys,
  ordersService,
  type Order,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";

const REASON_MAX = 500;
const TRACKING_MAX = 128;
/** Delivery receipt types and size (backend MaxReceiptBytes). */
export const RECEIPT_ACCEPT = "application/pdf,image/jpeg,image/png,image/webp";
export const RECEIPT_MAX_BYTES = 10 * 1024 * 1024;

/** Stores an updated order in the detail cache and refreshes the lists. */
export function useStoreOrder() {
  const qc = useQueryClient();
  return (order: Order) => {
    qc.setQueryData(orderKeys.detail(order.uuid), order);
    void qc.invalidateQueries({ queryKey: ["orders", "list"] });
  };
}

/**
 * Confirmation of a status action (detail page and list row actions,
 * TEC-374); cancel kinds ask for a reason. Shipping asks for the delivery
 * method and the tracking number, receiving takes an optional delivery
 * receipt (TEC-768).
 */
export function OrderActionDialog({
  order,
  action,
  onClose,
}: {
  order: Order | null;
  action: OrderAction | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const [reason, setReason] = useState("");
  const [method, setMethod] = useState("");
  const [tracking, setTracking] = useState("");
  const [receipt, setReceipt] = useState<File | null>(null);
  const shipping = action?.kind === "ship";
  const receiving = action?.kind === "receive";
  const methods = useQuery({
    queryKey: orderKeys.activeDeliveryMethods,
    queryFn: () => deliveryMethodsService.listActive(),
    enabled: shipping && order !== null,
  });
  const reset = () => {
    setReason("");
    setMethod("");
    setTracking("");
    setReceipt(null);
  };
  const close = () => {
    reset();
    onClose();
  };
  const mutation = useMutation({
    mutationFn: async (v: { order: Order; action: OrderAction }) => {
      const why = reason.trim() || undefined;
      let updated =
        v.action.kind === "ship"
          ? await ordersService.transition(v.order.uuid, v.action.target, why, {
              delivery_method_uuid: method,
              ...(tracking.trim() ? { tracking_no: tracking.trim() } : {}),
            })
          : await ordersService.transition(v.order.uuid, v.action.target, why);
      if (v.action.kind === "receive" && receipt) {
        try {
          updated = await ordersService.attachReceipt(v.order.uuid, receipt);
        } catch (err) {
          // The order is received; only the receipt upload failed.
          toast.error(
            orderErrorMessage(err, t, t("orders.shipping.receipt_failed")),
          );
        }
      }
      return updated;
    },
    onSuccess: (updated, v) => {
      store(updated);
      toast.success(t(`orders.actions.${v.action.kind}.success`));
      reset();
      onClose();
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.actions.failed"))),
  });
  const open = action !== null && order !== null;
  const reasonOk = !action?.reasonRequired || reason.trim().length >= 3;
  const shippingOk = !shipping || method !== "";
  const receiptTooLarge = receipt !== null && receipt.size > RECEIPT_MAX_BYTES;
  return (
    <Dialog open={open} onOpenChange={(o) => (!o ? close() : undefined)}>
      <DialogContent>
        {action && order ? (
          <>
            <DialogHeader>
              <DialogTitle>
                {t(`orders.actions.${action.kind}.title`)}
              </DialogTitle>
              <DialogDescription>
                {t(`orders.actions.${action.kind}.description`)}
              </DialogDescription>
            </DialogHeader>
            {action.destructive ? (
              <div className="space-y-1.5">
                <Label htmlFor="order-reason">
                  {action.reasonRequired
                    ? t("orders.actions.reason_required")
                    : t("orders.actions.reason_optional")}
                </Label>
                <Textarea
                  id="order-reason"
                  value={reason}
                  rows={3}
                  maxLength={REASON_MAX}
                  onChange={(e) => setReason(e.target.value)}
                />
              </div>
            ) : null}
            {shipping ? (
              <div className="space-y-3" data-testid="ship-fields">
                <div className="space-y-1.5">
                  <Label htmlFor="ship-method">
                    {t("orders.shipping.method")}
                  </Label>
                  <Select value={method} onValueChange={setMethod}>
                    <SelectTrigger
                      id="ship-method"
                      className="w-full"
                      data-testid="ship-method"
                    >
                      <SelectValue
                        placeholder={t("orders.shipping.method_placeholder")}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {(methods.data ?? []).map((m) => (
                        <SelectItem
                          key={m.uuid}
                          value={m.uuid}
                          data-testid="ship-method-option"
                        >
                          {m.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="ship-tracking">
                    {t("orders.shipping.tracking_no")}
                  </Label>
                  <Input
                    id="ship-tracking"
                    dir="ltr"
                    autoComplete="off"
                    value={tracking}
                    maxLength={TRACKING_MAX}
                    placeholder={t("orders.shipping.tracking_placeholder")}
                    onChange={(e) => setTracking(e.target.value)}
                  />
                </div>
              </div>
            ) : null}
            {receiving ? (
              <div className="space-y-1.5">
                <Label htmlFor="receive-receipt">
                  {t("orders.shipping.receipt_optional")}
                </Label>
                <Input
                  id="receive-receipt"
                  type="file"
                  accept={RECEIPT_ACCEPT}
                  data-testid="receive-receipt"
                  onChange={(e) => setReceipt(e.target.files?.[0] ?? null)}
                />
                {receiptTooLarge ? (
                  <p className="text-destructive text-xs">
                    {t("orders.shipping.receipt_too_large")}
                  </p>
                ) : null}
              </div>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={close}>
                {t("orders.actions.back")}
              </Button>
              <Button
                type="button"
                variant={action.destructive ? "destructive" : "default"}
                data-testid="action-confirm"
                disabled={
                  !reasonOk ||
                  !shippingOk ||
                  receiptTooLarge ||
                  mutation.isPending
                }
                onClick={() => mutation.mutate({ order, action })}
              >
                {t(`orders.actions.${action.kind}.confirm`)}
              </Button>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
