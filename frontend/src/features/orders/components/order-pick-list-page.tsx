"use client";

import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Printer } from "lucide-react";
import Link from "next/link";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { isRoll } from "@/features/orders/lib/form";
import {
  orderKeys,
  ordersService,
  type OrderPickRow,
} from "@/features/orders/services/orders.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function amount(
  r: Pick<OrderPickRow, "meters" | "quantity" | "product">,
  t: (k: string) => string,
): string {
  const parts: string[] = [];
  if (r.quantity != null) parts.push(`${r.quantity} ${t("orders.unit.pcs")}`);
  if (r.meters && isRoll(r.product.unit_type)) {
    parts.push(`${r.meters} ${t("orders.unit.m")}`);
  }
  return parts.join(" · ") || "—";
}

/**
 * Printable pick list of an order (TEC-768, print view; the DataTable rule
 * exempts print previews): assigned units at their location and, for lines
 * still missing units, the locations holding the product, in location
 * order. The browser prints it (window.print).
 */
export function OrderPickListPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const allowed = can(permissions.orders.ship);
  const query = useQuery({
    queryKey: orderKeys.pickList(uuid),
    queryFn: () => ordersService.pickList(uuid),
    enabled: allowed && Boolean(uuid),
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });
  const back = (
    <Button asChild variant="ghost" size="sm" className="print:hidden">
      <Link href={routes.tenant.orders.detail(slug, uuid)}>
        <ArrowLeft className="size-4 rtl:rotate-180" />
        {t("orders.picking.back_to_order")}
      </Link>
    </Button>
  );
  if (!allowed) {
    return (
      <div className="space-y-4">
        {back}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("orders.list.forbidden")}
        />
      </div>
    );
  }
  if (query.isLoading) return <Loading />;
  if (query.isError || !query.data) {
    return (
      <div className="space-y-4">
        {back}
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  const pl = query.data;
  return (
    <div className="space-y-6 print:space-y-3" data-testid="pick-list">
      <div className="flex flex-wrap items-center justify-between gap-2 print:hidden">
        {back}
        <Button
          type="button"
          onClick={() => window.print()}
          data-testid="pick-list-print"
        >
          <Printer className="size-4" />
          {t("orders.picking.print")}
        </Button>
      </div>
      <header className="space-y-1">
        <h1 className="text-xl font-semibold">
          {t("orders.picking.pick_list")} · <span dir="ltr">{pl.order_no}</span>
        </h1>
        <p className="text-muted-foreground text-sm">
          {pl.seller.name} → {pl.buyer.name} · {t(`orders.status.${pl.status}`)}{" "}
          · {format.dateTime(pl.generated_at)}
        </p>
      </header>
      <section className="space-y-2">
        <h2 className="text-sm font-semibold">{t("orders.picking.lines")}</h2>
        <ul className="grid gap-1 text-sm sm:grid-cols-2">
          {pl.lines.map((l) => (
            <li
              key={l.item_uuid}
              className="flex justify-between gap-2 border-b py-1"
            >
              <span>
                {l.product.name}{" "}
                <span className="text-muted-foreground text-xs" dir="ltr">
                  {l.product.sku}
                </span>
              </span>
              <span className="tabular-nums">
                {l.assigned} /{" "}
                {isRoll(l.product.unit_type)
                  ? `${l.meters ?? "0"} ${t("orders.unit.m")}`
                  : `${l.quantity ?? 0} ${t("orders.unit.pcs")}`}
              </span>
            </li>
          ))}
        </ul>
      </section>
      <section>
        {pl.rows.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("orders.picking.empty")}
          </p>
        ) : (
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="border-b-2 text-start">
                <th className="py-1 pe-2 text-start">
                  {t("orders.picking.location")}
                </th>
                <th className="py-1 pe-2 text-start">
                  {t("orders.form.product")}
                </th>
                <th className="py-1 pe-2 text-start">
                  {t("orders.picking.barcode")}
                </th>
                <th className="py-1 pe-2 text-end">
                  {t("orders.items.amount")}
                </th>
                <th className="w-10 py-1 text-center print:table-cell">✓</th>
              </tr>
            </thead>
            <tbody>
              {pl.rows.map((r, i) => (
                <tr
                  key={`${r.item_uuid}-${r.barcode ?? r.location_code ?? "x"}-${i}`}
                  className="border-b align-top"
                  data-testid="pick-row"
                  data-kind={r.kind}
                >
                  <td className="py-1 pe-2 font-mono text-xs" dir="ltr">
                    {r.location_code ?? t("orders.picking.no_location")}
                  </td>
                  <td className="py-1 pe-2">
                    {r.product.name}
                    <div className="text-muted-foreground text-xs" dir="ltr">
                      {r.product.sku}
                    </div>
                  </td>
                  <td className="py-1 pe-2 font-mono text-xs" dir="ltr">
                    {r.barcode ?? (
                      <span className="text-muted-foreground font-sans">
                        {t("orders.picking.stock_row")}
                      </span>
                    )}
                  </td>
                  <td className="py-1 pe-2 text-end tabular-nums">
                    {amount(r, t)}
                  </td>
                  <td className="py-1 text-center">
                    <span className="inline-block size-4 border" />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
