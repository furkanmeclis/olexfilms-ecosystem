"use client";

import { PackagePlus, Link2 } from "lucide-react";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { orderStatusTone } from "@/features/orders/lib/form";
import type { Order } from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1222: "Add supplementary shipment" on a shipped or delivered order;
 * opens the new order form prefilled from it (dealer and warehouse).
 */
export function AddSupplementButton({
  slug,
  order,
}: {
  slug: string;
  order: Order;
}) {
  const { t } = useLocale();
  if (!order.can_add_supplement) return null;
  return (
    <Button asChild variant="outline">
      <Link
        href={routes.tenant.orders.supplement(slug, order.uuid)}
        data-testid="order-add-supplement"
      >
        <PackagePlus className="size-4" />
        {t("orders.supplements.add")}
      </Link>
    </Button>
  );
}

/** TEC-1222: the list mark of a supplementary order. */
export function SupplementBadge({
  order,
}: {
  order: Pick<Order, "is_supplement">;
}) {
  const { t } = useLocale();
  if (!order.is_supplement) return null;
  return (
    <Badge variant="outline" data-testid="order-supplement-badge">
      {t("orders.supplements.badge")}
    </Badge>
  );
}

/**
 * TEC-1222: the main order of a supplementary order, or the supplementary
 * orders of a main order (nothing for an order with neither).
 */
export function OrderSupplementsCard({
  slug,
  order,
}: {
  slug: string;
  order: Order;
}) {
  const { t } = useLocale();
  const parent = order.parent_order;
  const rows = order.supplements ?? [];
  if (!parent && rows.length === 0) return null;
  return (
    <Card data-testid="order-supplements">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Link2 className="size-4" />
          {parent
            ? t("orders.supplements.parent")
            : t("orders.supplements.title_list")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-2">
          {(parent ? [parent] : rows).map((o) => (
            <li
              key={o.uuid}
              className="flex items-center justify-between gap-2 text-sm"
            >
              <Link
                href={routes.tenant.orders.detail(slug, o.uuid)}
                className="font-mono font-medium hover:underline"
                dir="ltr"
                data-testid={
                  parent ? "order-parent-link" : "order-supplement-link"
                }
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
      </CardContent>
    </Card>
  );
}
