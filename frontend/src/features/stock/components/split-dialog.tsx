"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import { LabelButton } from "@/features/warehouse/components/label-button";
import { stockErrorMessage } from "@/features/stock/lib/errors";
import { newSplitKey, splitPreview } from "@/features/stock/lib/stock-ops";
import {
  stockOpsService,
  unitLabelPath,
  type StockSplit,
} from "@/features/stock/services/stock-ops.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * "Split the roll" (TEC-184, TEC-763): cuts meters off a roll as a new
 * unit (`<barcode>-S<n>`), previews what stays on the roll and, once done,
 * offers the new unit's label PDF. One idempotency key per dialog keeps a
 * double submit from cutting twice.
 */
export function SplitDialog({
  slug,
  barcode,
  remaining,
  open,
  onOpenChange,
}: {
  slug: string;
  barcode: string;
  /** Remaining meters of the roll ("37.50"). */
  remaining: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [meters, setMeters] = useState("");
  const [reason, setReason] = useState("");
  const [touched, setTouched] = useState(false);
  const [key, setKey] = useState(newSplitKey);
  const [done, setDone] = useState<StockSplit | null>(null);

  const preview = splitPreview(remaining, meters);

  const reset = () => {
    setMeters("");
    setReason("");
    setTouched(false);
    setKey(newSplitKey());
    setDone(null);
  };

  const split = useMutation({
    mutationFn: (cut: string) =>
      stockOpsService.split({
        barcode,
        meters: cut,
        idempotency_key: key,
        ...(reason.trim() ? { reason: reason.trim() } : {}),
      }),
    onSuccess: (next) => {
      setDone(next);
      void qc.invalidateQueries({ queryKey: ["stock"] });
      appToast.success(t("stock.split.done"));
    },
    onError: (err) => {
      // A rejected cut writes nothing: a new attempt gets a new key.
      setKey(newSplitKey());
      appToast.error(stockErrorMessage(err, t, t("stock.split.failed")));
    },
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setTouched(true);
    if (preview.error !== null) return;
    split.mutate(preview.meters);
  };

  const close = () => {
    reset();
    onOpenChange(false);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent data-testid="split-dialog">
        <DialogHeader>
          <DialogTitle>{t("stock.split.title")}</DialogTitle>
          <DialogDescription>
            {t("stock.split.description", { barcode, remaining })}
          </DialogDescription>
        </DialogHeader>

        {done ? (
          <div className="space-y-3" data-testid="split-done">
            <p className="text-sm">
              {t("stock.split.result", {
                meters: done.meters,
                barcode: done.new_unit.barcode,
                left: done.source.remaining_meters,
              })}
            </p>
            <p className="font-mono text-lg font-semibold" dir="ltr">
              {done.new_unit.barcode}
            </p>
            <DialogFooter className="flex-wrap gap-2">
              <LabelButton
                path={
                  done.new_unit.label_url ??
                  unitLabelPath(done.new_unit.barcode)
                }
                filename={`${done.new_unit.barcode}.pdf`}
                variant="default"
                testId="split-label"
              />
              <Button asChild variant="outline">
                <Link
                  href={routes.tenant.stock.unit(slug, done.new_unit.barcode)}
                  data-testid="split-open-new"
                >
                  {t("stock.split.open_new")}
                </Link>
              </Button>
              <Button type="button" variant="ghost" onClick={close}>
                {t("common.close")}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <form className="space-y-4" onSubmit={onSubmit} noValidate>
            <div className="space-y-2">
              <Label htmlFor="split-meters">{t("stock.split.meters")}</Label>
              <Input
                id="split-meters"
                inputMode="decimal"
                autoComplete="off"
                value={meters}
                onChange={(e) => setMeters(e.target.value)}
                onBlur={() => setTouched(true)}
                placeholder="0.00"
                aria-invalid={touched && preview.error !== null}
              />
              {touched && preview.error ? (
                <p
                  role="alert"
                  className="text-destructive text-xs"
                  data-testid="split-error"
                >
                  {t(`stock.split.errors.${preview.error}`, { remaining })}
                </p>
              ) : null}
            </div>
            <p
              className="bg-muted rounded-md p-3 text-sm"
              data-testid="split-preview"
            >
              {preview.error === null
                ? t("stock.split.preview", {
                    meters: preview.meters,
                    left: preview.left,
                  })
                : t("stock.split.preview_empty", { remaining })}
            </p>
            <div className="space-y-2">
              <Label htmlFor="split-reason">{t("stock.split.reason")}</Label>
              <Textarea
                id="split-reason"
                value={reason}
                maxLength={2000}
                onChange={(e) => setReason(e.target.value)}
              />
            </div>
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
                disabled={split.isPending}
                data-testid="split-submit"
              >
                {t("stock.split.submit")}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
