"use client";

import { useQueries } from "@tanstack/react-query";

import { Button } from "@/components/ui/button";
import {
  ordersService,
  type OrderSide,
  type OrderStatus,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";

/** One queue of the seller's list (legacy STATUS_CHIPS): a status set. */
export type OrderQueue = {
  id: string;
  /** i18n key under `orders.list.queue.*`. */
  key: string;
  statuses: OrderStatus[];
};

export const ORDER_QUEUES: OrderQueue[] = [
  { id: "all", key: "orders.list.queue.all", statuses: [] },
  {
    id: "prepare",
    key: "orders.list.queue.prepare",
    statuses: ["approved", "processing"],
  },
  {
    id: "preparing",
    key: "orders.list.queue.preparing",
    statuses: ["preparing"],
  },
  { id: "ready", key: "orders.list.queue.ready", statuses: ["ready"] },
  { id: "shipped", key: "orders.list.queue.shipped", statuses: ["shipped"] },
  {
    id: "delivered",
    key: "orders.list.queue.delivered",
    statuses: ["delivered"],
  },
];

/** The queue whose status set equals the current status filter, if any. */
export function activeQueue(statusFilter: readonly string[]): string | null {
  const current = [...statusFilter].sort().join(",");
  const q = ORDER_QUEUES.find(
    (x) => [...x.statuses].sort().join(",") === current,
  );
  return q?.id ?? null;
}

/**
 * TEC-1032: queue chips of the orders list (to prepare / preparing / ready /
 * shipped / delivered) with live totals. A click replaces the status
 * filter; "all" clears it.
 */
export function OrderQueueChips({
  side,
  statusFilter,
  onPick,
}: {
  side: OrderSide;
  statusFilter: readonly string[];
  onPick: (statuses: OrderStatus[]) => void;
}) {
  const { t, format } = useLocale();
  const counts = useQueries({
    queries: ORDER_QUEUES.filter((q) => q.statuses.length > 0).map((q) => ({
      queryKey: ["orders", "queue-count", side, q.id] as const,
      queryFn: async () =>
        (
          await ordersService.list({
            side,
            status: q.statuses.join(","),
            limit: 1,
            offset: 0,
          })
        ).total,
      staleTime: 30_000,
    })),
  });
  const active = activeQueue(statusFilter);
  let i = 0;
  return (
    <div
      className="flex flex-wrap gap-2"
      role="group"
      data-testid="order-queues"
    >
      {ORDER_QUEUES.map((q) => {
        const count = q.statuses.length > 0 ? counts[i++]?.data : undefined;
        const isActive = active === q.id;
        return (
          <Button
            key={q.id}
            type="button"
            size="sm"
            variant={isActive ? "default" : "outline"}
            aria-pressed={isActive}
            data-testid="order-queue"
            data-queue={q.id}
            data-count={typeof count === "number" ? count : undefined}
            onClick={() => onPick(q.statuses)}
          >
            {t(q.key)}
            {typeof count === "number" ? (
              <span className="bg-background/20 ms-1 rounded-full px-1.5 text-xs tabular-nums">
                {format.number(count)}
              </span>
            ) : null}
          </Button>
        );
      })}
    </div>
  );
}
