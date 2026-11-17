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
  ReceiptCheckFields,
  receiptDiscrepancies,
  receiptMarksValid,
  type ReceiptMark,
} from "@/features/orders/components/order-partial";
import {
  shipmentDetails,
  shipmentDetailsValid,
} from "@/features/orders/lib/shipment";
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
  // TEC-1228: package count, desi and weight for the shipping note.
  const [packages, setPackages] = useState("");
  const [desi, setDesi] = useState("");
  const [weight, setWeight] = useState("");
  const [receipt, setReceipt] = useState<File | null>(null);
  // TEC-1227: missing / damaged units reported with the receipt.
  const [marks, setMarks] = useState<Record<string, ReceiptMark>>({});
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
    setPackages("");
    setDesi("");
    setWeight("");
    setReceipt(null);
    setMarks({});
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
              ...shipmentDetails(packages, desi, weight),
            })
          : v.action.kind === "receive" && receiptDiscrepancies(marks).length
            ? await ordersService.transition(
                v.order.uuid,
                v.action.target,
                why,
                undefined,
                receiptDiscrepancies(marks),
              )
            : await ordersService.transition(
                v.order.uuid,
                v.action.target,
                why,
              );
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
  const detailsOk = shipmentDetailsValid(packages, desi, weight);
  const shippingOk = !shipping || (method !== "" && detailsOk);
  const receiptTooLarge = receipt !== null && receipt.size > RECEIPT_MAX_BYTES;
  const marksOk =
    !receiving || order === null || receiptMarksValid(order, marks);
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
            {action.destructive || action.kind === "deliver" ? (
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
                <div className="grid grid-cols-3 gap-2">
                  <div className="space-y-1.5">
                    <Label htmlFor="ship-packages">
                      {t("orders.shipping.package_count")}
                    </Label>
                    <Input
                      id="ship-packages"
                      data-testid="ship-packages"
                      inputMode="numeric"
                      dir="ltr"
                      value={packages}
                      onChange={(e) => setPackages(e.target.value)}
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="ship-desi">
                      {t("orders.shipping.desi")}
                    </Label>
                    <Input
                      id="ship-desi"
                      data-testid="ship-desi"
                      inputMode="decimal"
                      dir="ltr"
                      value={desi}
                      onChange={(e) => setDesi(e.target.value)}
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="ship-weight">
                      {t("orders.shipping.weight_kg")}
                    </Label>
                    <Input
                      id="ship-weight"
                      data-testid="ship-weight"
                      inputMode="decimal"
                      dir="ltr"
                      value={weight}
                      onChange={(e) => setWeight(e.target.value)}
                    />
                  </div>
                </div>
                {detailsOk ? null : (
                  <p className="text-destructive text-sm">
                    {t("orders.shipping.details_invalid")}
                  </p>
                )}
              </div>
            ) : null}
            {receiving && order.status !== "received" ? (
              <ReceiptCheckFields
                order={order}
                marks={marks}
                onChange={setMarks}
              />
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
                  !marksOk ||
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
