"use client";

import { Route } from "lucide-react";
import Link from "next/link";

import { StatusChip } from "@/components/common/status-chip";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import type { components } from "@/generated/api";
import { useLocale } from "@/providers/locale-provider";

export type UnitTrace = components["schemas"]["StockUnitTrace"];

const sourceTone = {
  order: "success",
  stock: "default",
  unknown: "warning",
} as const;

/**
 * TEC-1006: where a unit came from and went — its origin warehouse, the
 * orders that carried it (to which dealer, when) and the services that used
 * it (with the customer). "Source unknown" marks a unit at or used by
 * another organization without an order (legacy Excel imports).
 */
export function UnitTraceCard({
  trace,
  slug,
}: {
  trace: UnitTrace;
  slug?: string;
}) {
  const { t, format } = useLocale();
  return (
    <Card data-testid="unit-trace">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <Route className="size-4" />
          {t("stock.trace.title")}
        </CardTitle>
        <StatusChip
          label={t(`stock.trace.source.${trace.source}`)}
          tone={sourceTone[trace.source]}
        />
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <p>
          <span className="text-muted-foreground">
            {t("stock.trace.origin")}:{" "}
          </span>
          {trace.origin_warehouse
            ? `${trace.origin_warehouse.code} · ${trace.origin_warehouse.name}`
            : "—"}
        </p>
        <section className="space-y-1">
          <h4 className="font-medium">{t("stock.trace.orders")}</h4>
          {trace.orders.length === 0 ? (
            <p className="text-muted-foreground">
              {trace.source === "unknown"
                ? t("stock.trace.no_order_unknown")
                : t("stock.trace.no_order")}
            </p>
          ) : (
            <ul className="space-y-1" data-testid="trace-orders">
              {trace.orders.map((o) => (
                <li key={o.uuid} className="flex flex-wrap gap-x-2">
                  {slug ? (
                    <Link
                      href={routes.tenant.orders.detail(slug, o.uuid)}
                      className="font-mono hover:underline"
                      dir="ltr"
                    >
                      {o.order_no}
                    </Link>
                  ) : (
                    <span className="font-mono" dir="ltr">
                      {o.order_no}
                    </span>
                  )}
                  <span>
                    {o.seller.name} → {o.buyer.name}
                  </span>
                  <span className="text-muted-foreground">
                    {format.dateTime(
                      o.received_at ?? o.shipped_at ?? o.created_at,
                    )}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
        <section className="space-y-1">
          <h4 className="font-medium">{t("stock.trace.services")}</h4>
          {trace.services.length === 0 ? (
            <p className="text-muted-foreground">
              {t("stock.trace.no_service")}
            </p>
          ) : (
            <ul className="space-y-1" data-testid="trace-services">
              {trace.services.map((s) => (
                <li key={s.uuid} className="flex flex-wrap gap-x-2">
                  {slug ? (
                    <Link
                      href={routes.tenant.services.detail(slug, s.uuid)}
                      className="font-mono hover:underline"
                      dir="ltr"
                    >
                      {s.service_no}
                    </Link>
                  ) : (
                    <span className="font-mono" dir="ltr">
                      {s.service_no}
                    </span>
                  )}
                  <span>{s.organization.name}</span>
                  <span>· {s.customer.name || "—"}</span>
                  {s.completed_at ? (
                    <span className="text-muted-foreground">
                      {format.dateTime(s.completed_at)}
                    </span>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </section>
      </CardContent>
    </Card>
  );
}
