"use client";

import { useQuery } from "@tanstack/react-query";
import { Undo2 } from "lucide-react";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { transferStatusTone } from "@/features/transfers/lib/transfers";
import {
  transferKeys,
  transfersService,
  type TransferListQuery,
} from "@/features/transfers/services/transfers.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** The order fields the card needs. */
export type OrderReturnsTarget = {
  uuid: string;
  status: string;
  role: string;
  buyer: { uuid: string };
};

const CLOSED = new Set(["draft", "cancelling", "cancelled"]);

/**
 * TEC-1085: the returns linked to an order on the order page, and "Create
 * return": the seller (transfers.approve) opens it in the buyer's name, the
 * buyer (transfers.request) its own return to the seller; both preselect
 * the order. Hidden without a transfer permission or when the transfers
 * module is off (the list answers an error).
 */
export function OrderReturnsCard({
  slug,
  order,
}: {
  slug: string;
  order: OrderReturnsTarget;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead =
    can(Permission.TransfersRequest) || can(Permission.TransfersApprove);
  const query: TransferListQuery = {
    kind: "return",
    order_uuid: order.uuid,
    limit: 50,
    offset: 0,
  };
  const returns = useQuery({
    queryKey: transferKeys.list(query),
    queryFn: () => transfersService.list(query),
    enabled: canRead,
    retry: false,
  });
  if (!canRead || returns.isError) return null;

  const open = !CLOSED.has(order.status);
  const createHref =
    open && order.role === "seller" && can(Permission.TransfersApprove)
      ? routes.tenant.transfers.createReturnFor(
          slug,
          order.buyer.uuid,
          order.uuid,
        )
      : open && order.role === "buyer" && can(Permission.TransfersRequest)
        ? `${routes.tenant.transfers.createReturn(slug)}?order=${encodeURIComponent(order.uuid)}`
        : null;
  const items = returns.data?.items ?? [];

  return (
    <Card data-testid="order-returns">
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2 text-base">
          <Undo2 className="size-4" />
          {t("transfers.order_returns.title")}
        </CardTitle>
        {createHref ? (
          <Button asChild size="sm" variant="outline">
            <Link href={createHref} data-testid="order-return-create">
              {t("transfers.order_returns.create")}
            </Link>
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        {returns.isLoading ? (
          <p className="text-muted-foreground text-sm">
            {t("transfers.list.loading")}
          </p>
        ) : items.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="order-returns-empty"
          >
            {t("transfers.order_returns.empty")}
          </p>
        ) : (
          <ul className="divide-y" data-testid="order-returns-list">
            {items.map((r) => (
              <li
                key={r.uuid}
                className="flex flex-wrap items-center justify-between gap-2 py-2"
              >
                <span className="min-w-0">
                  <Link
                    href={routes.tenant.transfers.detail(slug, r.uuid)}
                    className="font-medium hover:underline"
                    dir="ltr"
                  >
                    {r.transfer_no}
                  </Link>
                  <span className="text-muted-foreground block text-xs">
                    {t("transfers.order_returns.units", {
                      count: r.item_count,
                    })}{" "}
                    · {format.dateTime(r.created_at)}
                  </span>
                </span>
                <StatusChip
                  label={t(`transfers.status.${r.status}`)}
                  tone={transferStatusTone(r.status)}
                />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
