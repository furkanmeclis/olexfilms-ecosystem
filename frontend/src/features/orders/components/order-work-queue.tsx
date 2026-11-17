"use client";

import { useMutation } from "@tanstack/react-query";
import { Flame, Hand, Package, UserCheck } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useStoreOrder } from "@/features/orders/components/order-action-dialog";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  ordersService,
  type Order,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";

/** A YYYY-MM-DD due date shown in the user's locale. */
function useDueText() {
  const { format } = useLocale();
  return (due: string) => format.date(`${due}T12:00:00`);
}

/**
 * TEC-1228: urgent, due date (red when overdue) and the job holder of an
 * order — next to the order number in the list and in the detail header.
 */
export function OrderQueueBadges({
  order,
  showPicker = true,
}: {
  order: Order;
  showPicker?: boolean;
}) {
  const { t } = useLocale();
  const dueText = useDueText();
  return (
    <>
      {order.priority === "urgent" ? (
        <Badge variant="danger" data-testid="order-urgent">
          <Flame className="me-1 size-3" />
          {t("orders.queue.urgent")}
        </Badge>
      ) : null}
      {order.due_date ? (
        <Badge
          variant={order.is_overdue ? "danger" : "outline"}
          data-testid={order.is_overdue ? "order-overdue" : "order-due"}
        >
          {t(order.is_overdue ? "orders.queue.overdue" : "orders.queue.due", {
            date: dueText(order.due_date),
          })}
        </Badge>
      ) : null}
      {showPicker && order.picker ? (
        <Badge
          variant={order.claimed_by_me ? "success" : "secondary"}
          data-testid="order-picker"
        >
          <UserCheck className="me-1 size-3" />
          {order.claimed_by_me
            ? t("orders.queue.claimed_by_me")
            : t("orders.queue.claimed_by", { name: order.picker.name })}
        </Badge>
      ) : null}
    </>
  );
}

/**
 * TEC-1228: the work queue card of an order — priority and due date (edit
 * when allowed), who took the job (take / take over / release) and the
 * shipment details entered at shipping.
 */
export function OrderWorkCard({ order }: { order: Order }) {
  const { t, format } = useLocale();
  const dueText = useDueText();
  const store = useStoreOrder();
  const [editing, setEditing] = useState(false);
  const claim = useMutation({
    mutationFn: (takeover: boolean) =>
      ordersService.claim(order.uuid, takeover),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.queue.claimed"));
    },
    onError: (err) => {
      toast.error(orderErrorMessage(err, t, t("orders.queue.claim_failed")));
      // Someone else may have taken it meanwhile: show who.
      void ordersService
        .get(order.uuid)
        .then(store)
        .catch(() => undefined);
    },
  });
  const release = useMutation({
    mutationFn: () => ordersService.releaseClaim(order.uuid),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.queue.released"));
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.queue.claim_failed"))),
  });

  const hasShipment =
    order.package_count != null ||
    order.desi != null ||
    order.weight_kg != null;
  const showJob = order.can_claim || order.picker != null;
  if (
    !showJob &&
    !hasShipment &&
    !order.can_edit_schedule &&
    order.priority !== "urgent" &&
    !order.due_date
  ) {
    return null;
  }
  const busy = claim.isPending || release.isPending;
  const heldByOther = order.picker != null && !order.claimed_by_me;

  return (
    <Card data-testid="order-work-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Hand className="text-muted-foreground size-4" />
          {t("orders.queue.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <dl className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-0.5">
            <dt className="text-muted-foreground text-xs">
              {t("orders.queue.priority")}
            </dt>
            <dd data-testid="order-priority">
              {t(`orders.queue.priority_${order.priority}`)}
            </dd>
          </div>
          <div className="space-y-0.5">
            <dt className="text-muted-foreground text-xs">
              {t("orders.queue.due_date")}
            </dt>
            <dd
              className={order.is_overdue ? "text-destructive font-medium" : ""}
            >
              {order.due_date ? dueText(order.due_date) : "—"}
              {order.is_overdue ? ` · ${t("orders.queue.overdue_short")}` : ""}
            </dd>
          </div>
          {showJob ? (
            <div className="space-y-0.5 sm:col-span-2">
              <dt className="text-muted-foreground text-xs">
                {t("orders.queue.picker")}
              </dt>
              <dd data-testid="order-picker-name">
                {order.picker
                  ? `${order.picker.name}${
                      order.claimed_at
                        ? ` · ${format.dateTime(order.claimed_at)}`
                        : ""
                    }`
                  : t("orders.queue.unclaimed")}
              </dd>
            </div>
          ) : null}
          {hasShipment ? (
            <div className="space-y-0.5 sm:col-span-2">
              <dt className="text-muted-foreground flex items-center gap-1 text-xs">
                <Package className="size-3" />
                {t("orders.shipping.details")}
              </dt>
              <dd data-testid="order-shipment-details" dir="auto">
                {[
                  order.package_count != null
                    ? t("orders.shipping.packages_value", {
                        count: order.package_count,
                      })
                    : null,
                  order.desi != null
                    ? `${t("orders.shipping.desi")}: ${order.desi}`
                    : null,
                  order.weight_kg != null ? `${order.weight_kg} kg` : null,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </dd>
            </div>
          ) : null}
        </dl>
        {heldByOther && order.can_claim ? (
          <p
            className="text-muted-foreground text-xs"
            data-testid="claim-warning"
          >
            {t("orders.queue.held_warning", { name: order.picker?.name ?? "" })}
          </p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {order.can_claim && order.picker == null ? (
            <Button
              type="button"
              size="sm"
              data-testid="order-claim"
              disabled={busy}
              onClick={() => claim.mutate(false)}
            >
              {t("orders.queue.claim")}
            </Button>
          ) : null}
          {heldByOther && order.can_take_over ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              data-testid="order-take-over"
              disabled={busy}
              onClick={() => claim.mutate(true)}
            >
              {t("orders.queue.take_over")}
            </Button>
          ) : null}
          {order.picker != null &&
          (order.claimed_by_me || order.can_take_over) ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              data-testid="order-release"
              disabled={busy}
              onClick={() => release.mutate()}
            >
              {t("orders.queue.release")}
            </Button>
          ) : null}
          {order.can_edit_schedule ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              data-testid="order-edit-schedule"
              onClick={() => setEditing(true)}
            >
              {t("orders.queue.edit_schedule")}
            </Button>
          ) : null}
        </div>
      </CardContent>
      {editing ? (
        <ScheduleDialog order={order} onClose={() => setEditing(false)} />
      ) : null}
    </Card>
  );
}

function ScheduleDialog({
  order,
  onClose,
}: {
  order: Order;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const store = useStoreOrder();
  const [urgent, setUrgent] = useState(order.priority === "urgent");
  const [due, setDue] = useState(order.due_date ?? "");
  const save = useMutation({
    mutationFn: () =>
      ordersService.setSchedule(order.uuid, {
        priority: urgent ? "urgent" : "normal",
        due_date: due || null,
      }),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.queue.schedule_saved"));
      onClose();
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.queue.schedule_failed"))),
  });
  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("orders.queue.edit_schedule")}</DialogTitle>
          <DialogDescription>
            {t("orders.queue.schedule_hint")}
          </DialogDescription>
        </DialogHeader>
        <ScheduleFields
          urgent={urgent}
          onUrgent={setUrgent}
          due={due}
          onDue={setDue}
        />
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            data-testid="order-schedule-save"
            disabled={save.isPending}
            onClick={() => save.mutate()}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** TEC-1228: the urgent switch and the due date (order forms and dialog). */
export function ScheduleFields({
  urgent,
  onUrgent,
  due,
  onDue,
}: {
  urgent: boolean;
  onUrgent: (v: boolean) => void;
  due: string;
  onDue: (v: string) => void;
}) {
  const { t } = useLocale();
  return (
    <div
      className="grid gap-4 sm:grid-cols-2"
      data-testid="order-schedule-fields"
    >
      <div className="flex items-center gap-3">
        <Switch
          id="order-urgent"
          checked={urgent}
          onCheckedChange={onUrgent}
          data-testid="order-urgent-switch"
        />
        <Label htmlFor="order-urgent">{t("orders.queue.urgent_label")}</Label>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="order-due">{t("orders.queue.due_date")}</Label>
        <DatePicker id="order-due" value={due} onChange={onDue} />
      </div>
    </div>
  );
}
