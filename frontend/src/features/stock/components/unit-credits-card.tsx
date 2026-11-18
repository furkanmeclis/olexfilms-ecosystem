"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Coins, Plus } from "lucide-react";
import Link from "next/link";
import { useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { stockErrorMessage } from "@/features/stock/lib/errors";
import {
  stockOpsKeys,
  stockOpsService,
  type StockUnitCredits,
} from "@/features/stock/services/stock-ops.service";
import { useAuthStore } from "@/lib/auth/session-store";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

/** API bounds of a credit increase (TEC-1221). */
export const MAX_CREDIT_INCREASE = 1000;
export const MAX_CREDIT_REASON = 1000;

export type CreditIncreaseErrors = {
  amount?: string;
  reason?: string;
};

/** Client checks of the increase dialog, mirroring the API. */
export function creditIncreaseErrors(v: {
  amount: string;
  reason: string;
}): CreditIncreaseErrors {
  const errors: CreditIncreaseErrors = {};
  const amount = v.amount.trim();
  if (
    !/^\d{1,4}$/.test(amount) ||
    Number(amount) < 1 ||
    Number(amount) > MAX_CREDIT_INCREASE
  ) {
    errors.amount = "stock.credits.amount_invalid";
  }
  const reason = v.reason.trim();
  if (reason === "" || reason.length > MAX_CREDIT_REASON) {
    errors.reason = "stock.credits.reason_required";
  }
  return errors;
}

/** A signed delta: +2, -1, 0. */
function signed(n: number): string {
  return n > 0 ? `+${n}` : String(n);
}

function serviceTone(
  status: string,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "completed":
      return "success";
    case "cancelled":
      return "danger";
    case "draft":
      return "default";
    default:
      return "warning";
  }
}

/**
 * TEC-1221 (kredili kod): the usage credits of a serial piece unit — the
 * remaining and total credits, the services it was selected in and the
 * credit ledger. The platform super admin may add credits with a reason.
 * The ledger is a timeline (DataTable exception, AGENTS.md §6).
 */
export function UnitCreditsCard({
  credits,
  barcode,
  slug,
}: {
  credits: StockUnitCredits;
  barcode: string;
  slug?: string;
}) {
  const { t, format } = useLocale();
  const isSuperAdmin = useAuthStore((s) => Boolean(s.user?.isSuperAdmin));
  const [open, setOpen] = useState(false);

  return (
    <Card data-testid="unit-credits">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <Coins className="size-4" />
          {t("stock.credits.title")}
        </CardTitle>
        {isSuperAdmin ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            data-testid="credits-increase"
            onClick={() => setOpen(true)}
          >
            <Plus className="size-4" />
            {t("stock.credits.increase")}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <dl className="grid gap-3 sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("stock.credits.remaining")}
            </dt>
            <dd
              className="text-lg font-semibold tabular-nums"
              data-testid="credits-remaining"
            >
              {format.number(credits.credits_remaining)} /{" "}
              {format.number(credits.credits_total)}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("stock.credits.used_count")}
            </dt>
            <dd
              className="text-lg font-semibold tabular-nums"
              data-testid="credits-used"
            >
              {format.number(credits.used_count)}
            </dd>
          </div>
        </dl>

        <section className="space-y-1">
          <h4 className="font-medium">{t("stock.credits.services")}</h4>
          {credits.services.length === 0 ? (
            <p className="text-muted-foreground">
              {t("stock.credits.no_service")}
            </p>
          ) : (
            <ul className="space-y-1" data-testid="credits-services">
              {credits.services.map((s) => (
                <li
                  key={s.uuid}
                  className="flex flex-wrap items-center gap-x-2 gap-y-1"
                >
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
                  <StatusChip
                    label={t(`services.status.${s.status}`)}
                    tone={serviceTone(s.status)}
                  />
                  <span>
                    {s.credits_taken > 0
                      ? t("stock.credits.taken", {
                          credits: format.number(s.credits_taken),
                        })
                      : t("stock.credits.refunded")}
                  </span>
                  {s.finished_product ? (
                    <Badge variant="secondary" className="text-xs">
                      {t("stock.credits.finished")}
                    </Badge>
                  ) : null}
                  <span className="text-muted-foreground">
                    {format.dateTime(s.completed_at ?? s.created_at)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="space-y-1">
          <h4 className="font-medium">{t("stock.credits.movements")}</h4>
          {credits.movements.length === 0 ? (
            <p className="text-muted-foreground">
              {t("stock.credits.no_movement")}
            </p>
          ) : (
            <ol className="space-y-2" data-testid="credits-movements">
              {credits.movements.map((m) => (
                <li key={m.uuid} className="border-s-2 ps-3">
                  <div className="flex flex-wrap items-center gap-x-2">
                    <span className="font-medium">
                      {t(`stock.credits.type.${m.type}`)}
                    </span>
                    <span className="tabular-nums" dir="ltr">
                      {t("stock.credits.delta_remaining", {
                        delta: signed(m.remaining_delta),
                      })}
                    </span>
                    {m.total_delta !== 0 ? (
                      <span className="tabular-nums" dir="ltr">
                        {t("stock.credits.delta_total", {
                          delta: signed(m.total_delta),
                        })}
                      </span>
                    ) : null}
                    {m.service_uuid && m.service_no ? (
                      slug ? (
                        <Link
                          href={routes.tenant.services.detail(
                            slug,
                            m.service_uuid,
                          )}
                          className="font-mono hover:underline"
                          dir="ltr"
                        >
                          {m.service_no}
                        </Link>
                      ) : (
                        <span className="font-mono" dir="ltr">
                          {m.service_no}
                        </span>
                      )
                    ) : null}
                  </div>
                  <div className="text-muted-foreground text-xs">
                    {format.dateTime(m.created_at)}
                    {m.actor_name ? ` · ${m.actor_name}` : ""}
                    {m.reason ? ` · ${m.reason}` : ""}
                  </div>
                </li>
              ))}
            </ol>
          )}
        </section>
      </CardContent>
      {isSuperAdmin ? (
        <CreditIncreaseDialog
          open={open}
          onOpenChange={setOpen}
          unitUuid={credits.unit_uuid}
          barcode={barcode}
        />
      ) : null}
    </Card>
  );
}

/** TEC-1221: the super admin's "add credits" dialog (amount + reason). */
export function CreditIncreaseDialog({
  open,
  onOpenChange,
  unitUuid,
  barcode,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  unitUuid: string;
  barcode: string;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [amount, setAmount] = useState("1");
  const [reason, setReason] = useState("");
  const [errors, setErrors] = useState<CreditIncreaseErrors>({});

  const reset = () => {
    setAmount("1");
    setReason("");
    setErrors({});
  };

  const increase = useMutation({
    mutationFn: () =>
      stockOpsService.increaseUnitCredits(unitUuid, {
        amount: Number(amount.trim()),
        reason: reason.trim(),
      }),
    onSuccess: () => {
      appToast.success(t("stock.credits.increased"));
      void qc.invalidateQueries({ queryKey: stockOpsKeys.unit(barcode) });
      reset();
      onOpenChange(false);
    },
    onError: (err) =>
      appToast.error(
        stockErrorMessage(err, t, t("stock.credits.increase_failed")),
      ),
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    const next = creditIncreaseErrors({ amount, reason });
    setErrors(next);
    if (Object.keys(next).length > 0) return;
    increase.mutate();
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent data-testid="credits-dialog">
        <DialogHeader>
          <DialogTitle>{t("stock.credits.increase_title")}</DialogTitle>
          <DialogDescription>
            {t("stock.credits.increase_description", { barcode })}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={onSubmit} noValidate>
          <div className="space-y-2">
            <Label htmlFor="credits-amount">{t("stock.credits.amount")}</Label>
            <Input
              id="credits-amount"
              name="amount"
              inputMode="numeric"
              dir="ltr"
              autoComplete="off"
              maxLength={4}
              className="max-w-32"
              value={amount}
              aria-invalid={errors.amount ? true : undefined}
              onChange={(e) => setAmount(e.target.value)}
            />
            {errors.amount ? (
              <p role="alert" className="text-destructive text-xs">
                {t(errors.amount, { max: MAX_CREDIT_INCREASE })}
              </p>
            ) : null}
          </div>
          <div className="space-y-2">
            <Label htmlFor="credits-reason">{t("stock.credits.reason")}</Label>
            <Textarea
              id="credits-reason"
              name="reason"
              value={reason}
              maxLength={MAX_CREDIT_REASON}
              placeholder={t("stock.credits.reason_placeholder")}
              aria-invalid={errors.reason ? true : undefined}
              onChange={(e) => setReason(e.target.value)}
            />
            {errors.reason ? (
              <p role="alert" className="text-destructive text-xs">
                {t(errors.reason)}
              </p>
            ) : null}
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
              disabled={increase.isPending}
              data-testid="credits-submit"
            >
              {t("stock.credits.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
