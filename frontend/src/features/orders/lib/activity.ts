import type { ActivityStep } from "@/components/common/activity-timeline";
import type { Order } from "@/features/orders/services/orders.service";

/** Order steps that are not a status move (TEC-1054 / TEC-1060). */
export const ORDER_ACTIVITY_ACTIONS = [
  "created",
  "unit_assigned",
  "unit_unassigned",
  "updated",
] as const;

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

/** The label of an order step: an activity, else the status moved to. */
export function orderActivityLabel(t: Translate, action: string): string {
  return (ORDER_ACTIVITY_ACTIONS as readonly string[]).includes(action)
    ? t(`orders.activity.${action}`)
    : t(`orders.status.${action}`);
}

type Formatter = {
  number: (v: number, o?: Intl.NumberFormatOptions) => string;
};

/** The order timeline as activity steps (TEC-1060). */
export function orderTimelineSteps(
  order: Order,
  t: Translate,
  format: Formatter,
): ActivityStep[] {
  // An older response without a timeline: the status history, no users.
  if (!order.timeline) {
    return (order.history ?? []).map((h, i) => ({
      key: `${h.created_at}-${i}`,
      at: h.created_at,
      user: null,
      label: h.from_status
        ? `${t(`orders.status.${h.from_status}`)} → ${t(`orders.status.${h.to_status}`)}`
        : t(`orders.status.${h.to_status}`),
      detail: h.reason ?? undefined,
    }));
  }
  return order.timeline.map((e, i) => {
    const parts: string[] = [];
    if (e.product) parts.push(e.product);
    if (e.barcode) parts.push(e.barcode);
    if (e.quantity != null) {
      parts.push(
        t("orders.activity.quantity", { count: format.number(e.quantity) }),
      );
    }
    if (e.meters) {
      parts.push(
        t("orders.list.quantity_meters", {
          meters: format.number(Number(e.meters), {
            maximumFractionDigits: 2,
          }),
        }),
      );
    }
    if (e.lines != null) {
      parts.push(t("orders.activity.lines", { count: format.number(e.lines) }));
    }
    // TEC-1056: the lines an edit added, changed or removed.
    for (const c of e.changes ?? []) {
      if (c.before == null && c.after != null) {
        parts.push(
          t("orders.activity.change_added", {
            product: c.product,
            after: c.after,
          }),
        );
      } else if (c.after == null && c.before != null) {
        parts.push(
          t("orders.activity.change_removed", {
            product: c.product,
            before: c.before,
          }),
        );
      } else {
        parts.push(
          t("orders.activity.change_changed", {
            product: c.product,
            before: c.before ?? "",
            after: c.after ?? "",
          }),
        );
      }
    }
    if (e.reason) parts.push(e.reason);
    return {
      key: `${e.at}-${i}`,
      at: e.at,
      user: e.user?.name ?? null,
      label: orderActivityLabel(t, e.action),
      detail: parts.length ? parts.join(" · ") : undefined,
    };
  });
}
