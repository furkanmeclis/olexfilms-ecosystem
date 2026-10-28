"use client";

import { ChevronDown } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Money } from "@/features/accounting/components/shared";
import type { CariStatementLine } from "@/features/accounting/services/accounting.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type StatementOrder = NonNullable<CariStatementLine["order"]>;
type StatementOrderItem = StatementOrder["items"][number];

/** "2 pcs" / "12.50 m" of an order line. */
function ItemAmount({ item }: { item: StatementOrderItem }) {
  const { t, format } = useLocale();
  if (item.meters != null) {
    return (
      <>
        {t("accounting.statement.order_meters", {
          n: format.number(Number(item.meters)),
        })}
      </>
    );
  }
  return (
    <>
      {t("accounting.statement.order_pieces", {
        n: format.number(item.quantity ?? 0),
      })}
    </>
  );
}

/**
 * TEC-1035: the order behind a statement row: a toggle that expands its
 * lines (product, quantity/meters, unit price, line total) and the barcodes
 * that went to the buyer with each line.
 */
export function StatementOrderDetail({ order }: { order: StatementOrder }) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const barcodes = order.items.reduce((n, it) => n + it.units.length, 0);
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      data-testid="statement-order"
      data-order={order.uuid}
    >
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="link"
          size="sm"
          className="h-auto px-0 text-xs"
          data-testid="statement-order-toggle"
          aria-label={t("accounting.statement.order_toggle", {
            no: order.order_no,
          })}
        >
          <ChevronDown
            className={cn(
              "size-3.5 transition-transform",
              open && "rotate-180",
            )}
          />
          {t("accounting.statement.order_summary", {
            no: order.order_no,
            items: order.items.length,
            barcodes,
          })}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <ul
          className="mt-2 space-y-2 border-s ps-3"
          data-testid="statement-order-items"
        >
          {order.items.map((it) => (
            <li
              key={it.product_uuid}
              className="text-xs"
              data-testid="statement-order-item"
            >
              <div className="font-medium">
                {it.product_name}
                {it.product_sku ? (
                  <span className="text-muted-foreground ms-1 font-normal">
                    ({it.product_sku})
                  </span>
                ) : null}
              </div>
              <div className="text-muted-foreground flex flex-wrap gap-x-2">
                <ItemAmount item={it} />
                <span>×</span>
                <Money amount={it.unit_price} currency={order.currency} />
                <span>=</span>
                <Money
                  amount={it.line_total}
                  currency={order.currency}
                  className="text-foreground font-medium"
                />
              </div>
              {it.units.length > 0 ? (
                <div
                  className="mt-1 flex flex-wrap gap-1"
                  aria-label={t("accounting.statement.barcodes")}
                >
                  {it.units.map((u) => (
                    <Badge
                      key={u.uuid}
                      variant="outline"
                      className="font-mono text-[11px]"
                      data-testid="statement-order-barcode"
                    >
                      {u.barcode}
                      {u.meters != null
                        ? ` · ${t("accounting.statement.order_meters", { n: u.meters })}`
                        : ""}
                    </Badge>
                  ))}
                </div>
              ) : (
                <div className="text-muted-foreground mt-1">
                  {t("accounting.statement.no_barcodes")}
                </div>
              )}
            </li>
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  );
}
