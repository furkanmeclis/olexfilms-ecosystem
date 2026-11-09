"use client";

import { useQuery } from "@tanstack/react-query";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { PickableUnit } from "@/features/transfers/lib/transfers";
import {
  transferKeys,
  transfersService,
  type StockReturnOrderUnit,
} from "@/features/transfers/services/transfers.service";
import { useLocale } from "@/providers/locale-provider";

const NONE = "none";

/** The picker row of an order unit: what the giver may still return. */
export function orderUnitPickable(u: StockReturnOrderUnit): PickableUnit {
  return {
    barcode: u.barcode,
    unit_kind: u.unit_kind,
    quantity: Number(u.returnable),
    remaining_meters: u.meters,
    product: { name: u.product.name },
  };
}

/**
 * TEC-1085: the order a return refers to (the parent's sale to the giver)
 * and the order's units offered for picking. fromOrgUuid is the child of a
 * return the parent opens, "" for the active organization's own return.
 * A unit the giver no longer holds is shown but cannot be picked.
 */
export function ReturnOrderPicker({
  fromOrgUuid,
  orderUuid,
  onOrderChange,
  picked,
  onToggle,
  enabled,
}: {
  fromOrgUuid: string;
  orderUuid: string;
  onOrderChange: (uuid: string) => void;
  picked: Set<string>;
  onToggle: (u: PickableUnit) => void;
  enabled: boolean;
}) {
  const { t } = useLocale();
  const orders = useQuery({
    queryKey: transferKeys.returnOrders(fromOrgUuid),
    queryFn: () => transfersService.returnOrders(fromOrgUuid || undefined),
    enabled,
  });
  const units = useQuery({
    queryKey: transferKeys.returnOrderUnits(orderUuid, fromOrgUuid),
    queryFn: () =>
      transfersService.returnOrderUnits(orderUuid, fromOrgUuid || undefined),
    enabled: enabled && orderUuid !== "",
  });
  const list = orders.data?.items ?? [];
  // A preselected order (from the order page) stays selectable while the
  // list loads.
  const known = orderUuid === "" || list.some((o) => o.uuid === orderUuid);

  return (
    <Card data-testid="return-order-picker">
      <CardHeader>
        <CardTitle>{t("transfers.return.order")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="return-order">{t("transfers.return.order")}</Label>
          <Select
            value={orderUuid === "" ? NONE : orderUuid}
            onValueChange={(v) => onOrderChange(v === NONE ? "" : v)}
          >
            <SelectTrigger
              id="return-order"
              data-testid="return-order"
              className="w-full sm:w-96"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE}>
                {t("transfers.return.order_none")}
              </SelectItem>
              {!known ? (
                <SelectItem value={orderUuid}>{orderUuid}</SelectItem>
              ) : null}
              {list.map((o) => (
                <SelectItem key={o.uuid} value={o.uuid}>
                  {o.order_no}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            {!orders.isLoading && list.length === 0
              ? t("transfers.return.no_orders")
              : t("transfers.return.order_hint")}
          </p>
        </div>
        {orderUuid !== "" ? (
          <div className="space-y-2">
            <p className="text-sm font-medium">
              {t("transfers.return.order_units")}
            </p>
            {units.isError ? (
              <p className="text-destructive text-sm">
                {t("common.error_generic")}
              </p>
            ) : units.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("transfers.list.loading")}
              </p>
            ) : (units.data?.items ?? []).length === 0 ? (
              <p
                className="text-muted-foreground text-sm"
                data-testid="return-order-units-empty"
              >
                {t("transfers.return.order_units_empty")}
              </p>
            ) : (
              <ul
                className="divide-y rounded-md border"
                data-testid="return-order-units"
              >
                {(units.data?.items ?? []).map((u) => {
                  const disabled = u.returnable <= 0;
                  const id = `return-unit-${u.unit_uuid}`;
                  return (
                    <li
                      key={u.unit_uuid}
                      className="flex items-center gap-2 p-2"
                      data-testid="return-order-unit"
                    >
                      <Checkbox
                        id={id}
                        checked={picked.has(u.barcode)}
                        disabled={disabled}
                        onCheckedChange={() => onToggle(orderUnitPickable(u))}
                      />
                      <Label htmlFor={id} className="min-w-0 flex-1">
                        <span className="block font-mono text-sm" dir="ltr">
                          {u.barcode}
                        </span>
                        <span className="text-muted-foreground block truncate text-xs">
                          {u.product.name}
                        </span>
                      </Label>
                      {disabled ? (
                        <span className="text-muted-foreground text-xs">
                          {t("transfers.return.not_held")}
                        </span>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
