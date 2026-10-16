"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useCallback, useRef, useState, type FormEvent } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { stockErrorMessage } from "@/features/stock/lib/errors";
import {
  reclassFormErrors,
  type OperableUnit,
  type ReclassFormErrors,
} from "@/features/stock/lib/stock-ops";
import {
  stockOpsService,
  type StockReclassification,
} from "@/features/stock/services/stock-ops.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Product = NonNullable<OperableUnit["product"]>;

/**
 * "Fix the product" (TEC-157, TEC-763): a request to move a unit to
 * another product of the same unit type; the barcode is kept and nothing
 * moves until a center user approves it.
 */
export function ReclassifyDialog({
  slug,
  barcode,
  product,
  open,
  onOpenChange,
}: {
  slug: string;
  barcode: string;
  product: Product;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [target, setTarget] = useState<ComboboxOption | null>(null);
  const [reason, setReason] = useState("");
  const [errors, setErrors] = useState<ReclassFormErrors>({});
  const [created, setCreated] = useState<StockReclassification | null>(null);
  const seen = useRef(new Map<string, ComboboxOption>());

  const reset = () => {
    setTarget(null);
    setReason("");
    setErrors({});
    setCreated(null);
  };

  // Active products of the same unit type, serial barcodes only (the
  // backend refuses another unit type or a fixed barcode product).
  const loadProducts = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await catalogService.listProducts({
        q: q.trim() || undefined,
        unit_type: product.unit_type,
        uses_fixed_barcode: false,
        active: true,
        limit: 20,
      });
      const options = page.items
        .filter((p) => p.uuid !== product.uuid)
        .map((p) => ({ value: p.uuid, label: p.name, description: p.sku }));
      for (const option of options) seen.current.set(option.value, option);
      return options;
    },
    [product.unit_type, product.uuid],
  );

  const request = useMutation({
    mutationFn: (body: { to_product_uuid: string; reason: string }) =>
      stockOpsService.requestReclassification({ barcode, ...body }),
    onSuccess: (next) => {
      setCreated(next);
      void qc.invalidateQueries({ queryKey: ["stock"] });
      appToast.success(t("stock.reclass.requested"));
    },
    onError: (err) =>
      appToast.error(stockErrorMessage(err, t, t("stock.reclass.failed"))),
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    const next = reclassFormErrors({ target: target?.value ?? "", reason });
    setErrors(next);
    if (Object.keys(next).length > 0 || !target) return;
    request.mutate({ to_product_uuid: target.value, reason: reason.trim() });
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent data-testid="reclass-dialog">
        <DialogHeader>
          <DialogTitle>{t("stock.reclass.title")}</DialogTitle>
          <DialogDescription>
            {t("stock.reclass.description", { barcode })}
          </DialogDescription>
        </DialogHeader>

        {created ? (
          <div className="space-y-3" data-testid="reclass-created">
            <p className="text-sm">
              {t("stock.reclass.created", {
                from: created.from_product.name,
                to: created.to_product.name,
              })}
            </p>
            <DialogFooter>
              <Button asChild variant="outline">
                <Link
                  href={routes.tenant.stock.reclassification(
                    slug,
                    created.uuid,
                  )}
                >
                  {t("stock.reclass.open_request")}
                </Link>
              </Button>
              <Button
                type="button"
                onClick={() => {
                  reset();
                  onOpenChange(false);
                }}
              >
                {t("common.close")}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <form className="space-y-4" onSubmit={onSubmit} noValidate>
            <div className="space-y-1 text-sm">
              <span className="text-muted-foreground">
                {t("stock.reclass.current")}
              </span>{" "}
              <span className="font-medium">{product.name}</span>{" "}
              <span className="text-muted-foreground font-mono text-xs">
                {product.sku}
              </span>
            </div>
            <div className="space-y-2" data-testid="reclass-target">
              <Label htmlFor="reclass-target">
                {t("stock.reclass.target")}
              </Label>
              <AsyncCombobox
                id="reclass-target"
                value={target?.value ?? ""}
                options={target ? [target] : undefined}
                loadOptions={loadProducts}
                onValueChange={(value) =>
                  setTarget(
                    value
                      ? (seen.current.get(value) ?? { value, label: value })
                      : null,
                  )
                }
                placeholder={t("stock.reclass.target_placeholder")}
                searchPlaceholder={t("stock.filters.product_search")}
                emptyText={t("stock.reclass.target_none")}
              />
              {errors.target ? (
                <p role="alert" className="text-destructive text-xs">
                  {t(errors.target)}
                </p>
              ) : null}
            </div>
            <div className="space-y-2">
              <Label htmlFor="reclass-reason">
                {t("stock.reclass.reason")}
              </Label>
              <Textarea
                id="reclass-reason"
                value={reason}
                maxLength={2000}
                onChange={(e) => setReason(e.target.value)}
                placeholder={t("stock.reclass.reason_placeholder")}
              />
              {errors.reason ? (
                <p role="alert" className="text-destructive text-xs">
                  {t(errors.reason)}
                </p>
              ) : null}
            </div>
            <p className="text-muted-foreground text-xs">
              {t("stock.reclass.hint")}
            </p>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={request.isPending}
                data-testid="reclass-submit"
              >
                {t("stock.reclass.submit")}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
