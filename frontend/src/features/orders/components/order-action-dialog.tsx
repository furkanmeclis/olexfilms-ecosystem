"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
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
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { OrderAction } from "@/features/orders/lib/access";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  orderKeys,
  ordersService,
  type Order,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";

const REASON_MAX = 500;

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
 * TEC-374); cancel kinds ask for a reason.
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
  const mutation = useMutation({
    mutationFn: (v: { order: Order; action: OrderAction }) =>
      ordersService.transition(
        v.order.uuid,
        v.action.target,
        reason.trim() || undefined,
      ),
    onSuccess: (updated, v) => {
      store(updated);
      toast.success(t(`orders.actions.${v.action.kind}.success`));
      setReason("");
      onClose();
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.actions.failed"))),
  });
  const open = action !== null && order !== null;
  const reasonOk = !action?.reasonRequired || reason.trim().length >= 3;
  return (
    <Dialog open={open} onOpenChange={(o) => (!o ? onClose() : undefined)}>
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
            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("orders.actions.back")}
              </Button>
              <Button
                type="button"
                variant={action.destructive ? "destructive" : "default"}
                data-testid="action-confirm"
                disabled={!reasonOk || mutation.isPending}
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
