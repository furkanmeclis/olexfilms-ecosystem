"use client";

import { useMutation } from "@tanstack/react-query";
import { ListChecks, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useStoreOrder } from "@/features/orders/components/order-action-dialog";
import { ProductSearch } from "@/features/orders/components/product-search";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  addLine,
  formIsValid,
  isRoll,
  linesBelowAssigned,
  linesFromOrder,
  toItemInputs,
  validateOrderForm,
  type OrderFormLine,
} from "@/features/orders/lib/form";
import {
  ordersService,
  type Order,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1056 (RV-5.5 / 5.9): the seller's sales or warehouse staff fix the
 * lines of an accepted order before it is ready — add a product (the
 * plexiglass and sign sets of a new dealer), change an amount, remove a
 * line. No price is typed: a new line is priced by the server. A line may
 * not go below its assigned units (unassign first); a removed line frees
 * its barcodes.
 */
export function OrderLinesEditor({
  order,
  onClose,
}: {
  order: Order;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const store = useStoreOrder();
  const [lines, setLines] = useState<OrderFormLine[]>(() =>
    linesFromOrder(order),
  );
  const [touched, setTouched] = useState(false);
  const errors = useMemo(() => validateOrderForm(lines), [lines]);
  const below = useMemo(() => linesBelowAssigned(order, lines), [order, lines]);
  const valid = formIsValid(errors) && below.size === 0;
  const picked = useMemo(
    () => new Set(lines.map((l) => l.product_uuid)),
    [lines],
  );
  const assignedOf = (productUuid: string) =>
    Number(
      order.items?.find((i) => i.product.uuid === productUuid)?.assigned ?? 0,
    );

  const save = useMutation({
    mutationFn: () =>
      ordersService.replaceItems(order.uuid, toItemInputs(lines)),
    onSuccess: (updated) => {
      store(updated);
      toast.success(t("orders.edit_lines.saved"));
      onClose();
    },
    onError: (err) =>
      toast.error(orderErrorMessage(err, t, t("orders.edit_lines.failed"))),
  });

  const update = (uuid: string, amount: string) =>
    setLines((ls) =>
      ls.map((l) => (l.product_uuid === uuid ? { ...l, amount } : l)),
    );

  return (
    <Card data-testid="order-lines-editor">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ListChecks className="size-4" />
          {t("orders.edit_lines.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-muted-foreground text-sm">
          {t("orders.edit_lines.hint")}
        </p>
        <ProductSearch
          picked={picked}
          onPick={(p) => setLines((ls) => addLine(ls, p))}
        />
        <div className="space-y-3">
          {lines.length === 0 ? (
            <p
              className={
                touched
                  ? "text-destructive text-sm"
                  : "text-muted-foreground text-sm"
              }
              data-testid="lines-empty"
            >
              {t("orders.form.lines_empty")}
            </p>
          ) : (
            lines.map((line) => {
              const err = errors.lines[line.product_uuid];
              const isBelow = below.has(line.product_uuid);
              const assigned = assignedOf(line.product_uuid);
              const id = `edit-line-${line.product_uuid}`;
              return (
                <div
                  key={line.product_uuid}
                  className="grid gap-3 rounded-lg border p-3 sm:grid-cols-[minmax(0,3fr)_minmax(7rem,1fr)_auto] sm:items-end"
                  data-testid="edit-line"
                  data-sku={line.sku}
                >
                  <div className="min-w-0 space-y-1">
                    <div className="font-medium">{line.name}</div>
                    <div className="text-muted-foreground text-xs" dir="ltr">
                      {line.sku}
                    </div>
                    {assigned > 0 ? (
                      <div
                        className="text-muted-foreground text-xs"
                        data-testid="edit-line-assigned"
                      >
                        {t("orders.edit_lines.assigned", {
                          amount: format.number(assigned, {
                            maximumFractionDigits: 2,
                          }),
                        })}
                      </div>
                    ) : null}
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor={id} className="text-xs">
                      {isRoll(line.unit_type)
                        ? t("orders.form.meters")
                        : t("orders.form.quantity")}
                    </Label>
                    <Input
                      id={id}
                      name="amount"
                      inputMode={isRoll(line.unit_type) ? "decimal" : "numeric"}
                      dir="ltr"
                      value={line.amount}
                      aria-invalid={
                        (touched && Boolean(err)) || isBelow || undefined
                      }
                      onChange={(e) =>
                        update(line.product_uuid, e.target.value)
                      }
                    />
                    {touched && err ? (
                      <p
                        className="text-destructive text-xs"
                        data-testid="line-error"
                      >
                        {t(`orders.form.errors.${err}`)}
                      </p>
                    ) : isBelow ? (
                      <p
                        className="text-destructive text-xs"
                        data-testid="line-below-assigned"
                      >
                        {t("orders.edit_lines.below_assigned")}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex items-start justify-end">
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      aria-label={t("orders.form.remove")}
                      data-testid="edit-line-remove"
                      onClick={() =>
                        setLines((ls) =>
                          ls.filter(
                            (l) => l.product_uuid !== line.product_uuid,
                          ),
                        )
                      }
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </div>
              );
            })
          )}
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={save.isPending}
            data-testid="edit-lines-cancel"
            onClick={onClose}
          >
            {t("orders.edit_lines.cancel")}
          </Button>
          <Button
            type="button"
            disabled={save.isPending}
            data-testid="edit-lines-save"
            onClick={() => {
              setTouched(true);
              if (valid) save.mutate();
            }}
          >
            {t("orders.edit_lines.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
