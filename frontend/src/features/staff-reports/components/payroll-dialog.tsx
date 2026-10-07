"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
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
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import { Money, FieldError } from "@/features/accounting/components/shared";
import {
  isFutureDay,
  isSalaryConflict,
  isValidPeriod,
  payrollAlreadyRan,
  payrollPreview,
  periodDate,
  periodOf,
  todayIso,
} from "@/features/staff-reports/lib/staff";
import {
  staffReportsKeys,
  staffReportsService,
  type StaffPayrollResult,
  type StaffProfile,
} from "@/features/staff-reports/services/staff-reports.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Outcome =
  | { kind: "done"; result: StaffPayrollResult }
  | { kind: "already"; period: string };

/**
 * Month-end salaries (POST /v1/staff-payments/payroll): pick the period
 * and the payment day, preview the active staff and their salaries,
 * confirm. A payment day ahead writes planned salaries that are booked on
 * that day (TEC-381). The run is
 * idempotent, so a period that already ran shows the "already ran"
 * message (a 409 or a run that created nothing).
 */
export function PayrollDialog({
  orgUuid,
  open,
  staff,
  onOpenChange,
}: {
  orgUuid: string;
  open: boolean;
  staff: readonly StaffProfile[];
  onOpenChange: (open: boolean) => void;
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const [period, setPeriod] = useState(() => periodOf());
  const [paidOn, setPaidOn] = useState(() => todayIso());
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const preview = useMemo(() => payrollPreview(staff), [staff]);
  const validPeriod = isValidPeriod(period);

  const run = useMutation({
    mutationFn: (p: string) =>
      staffReportsService.runPayroll(p, paidOn || undefined),
    onSuccess: async (result) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: staffReportsKeys.all(orgUuid),
        }),
        queryClient.invalidateQueries({
          queryKey: accountingKeys.all(orgUuid),
        }),
      ]);
      if (payrollAlreadyRan(result)) {
        setOutcome({ kind: "already", period: result.period });
        return;
      }
      setOutcome({ kind: "done", result });
      appToast.success(
        t(
          result.items.some((p) => p.status === "planned")
            ? "staff_reports.payroll.done_planned"
            : "staff_reports.payroll.done",
          { n: format.number(result.created) },
        ),
      );
    },
    onError: (error: unknown, p) => {
      if (isSalaryConflict(error)) {
        setOutcome({ kind: "already", period: p });
        return;
      }
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      );
    },
  });

  const change = (next: boolean) => {
    if (!next) {
      setOutcome(null);
      run.reset();
    }
    onOpenChange(next);
  };

  const monthLabel = (p: string) =>
    isValidPeriod(p)
      ? format.dateParts(periodDate(p), { year: "numeric", month: "long" })
      : p;

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent className="sm:max-w-lg" data-testid="payroll-dialog">
        <DialogHeader>
          <DialogTitle>{t("staff_reports.payroll.title")}</DialogTitle>
          <DialogDescription>
            {t("staff_reports.payroll.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="payroll-period">
            {t("staff_reports.fields.period")}
          </Label>
          <Input
            id="payroll-period"
            name="period"
            type="month"
            dir="ltr"
            className="w-44"
            value={period}
            aria-invalid={validPeriod ? undefined : true}
            onChange={(e) => {
              setPeriod(e.target.value);
              setOutcome(null);
            }}
          />
          <FieldError
            id="payroll-period-error"
            message={
              validPeriod ? undefined : t("staff_reports.validation.period")
            }
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="payroll-paid-on">
            {t("staff_reports.fields.paid_on")}
          </Label>
          <DatePicker
            id="payroll-paid-on"
            value={paidOn}
            onChange={(v) => {
              setPaidOn(v);
              setOutcome(null);
            }}
          />
          {isFutureDay(paidOn) ? (
            <p
              className="text-muted-foreground text-xs"
              data-testid="payroll-planned-hint"
            >
              {t("staff_reports.planned.future_hint")}
            </p>
          ) : null}
        </div>

        {outcome?.kind === "already" ? (
          <Alert variant="destructive" data-testid="payroll-already">
            <AlertDescription>
              {t("staff_reports.payroll.already_ran", {
                period: monthLabel(outcome.period),
              })}
            </AlertDescription>
          </Alert>
        ) : null}
        {outcome?.kind === "done" ? (
          <Alert data-testid="payroll-result">
            <AlertDescription>
              {t("staff_reports.payroll.result", {
                created: format.number(outcome.result.created),
                skipped: format.number(outcome.result.skipped),
              })}
            </AlertDescription>
          </Alert>
        ) : null}

        <div className="space-y-2" data-testid="payroll-preview">
          <p className="text-sm font-medium">
            {t("staff_reports.payroll.preview", {
              period: monthLabel(period),
            })}
          </p>
          {preview.payable.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("staff_reports.payroll.empty")}
            </p>
          ) : (
            <ul className="divide-y rounded-md border text-sm">
              {preview.payable.map((s) => (
                <li
                  key={s.uuid}
                  className="flex items-center justify-between gap-3 px-3 py-2"
                  data-testid="payroll-row"
                  data-staff={s.uuid}
                >
                  <span className="min-w-0 truncate">
                    {s.name}
                    {s.title ? (
                      <span className="text-muted-foreground">
                        {" "}
                        · {s.title}
                      </span>
                    ) : null}
                  </span>
                  <Money amount={s.monthly_salary ?? 0} currency={s.currency} />
                </li>
              ))}
              {preview.noSalary.map((s) => (
                <li
                  key={s.uuid}
                  className="text-muted-foreground flex items-center justify-between gap-3 px-3 py-2"
                  data-testid="payroll-row-skipped"
                  data-staff={s.uuid}
                >
                  <span className="min-w-0 truncate">{s.name}</span>
                  <span className="text-xs">
                    {t("staff_reports.payroll.no_salary")}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {preview.payable.length && preview.currency ? (
            <p className="flex justify-between text-sm font-semibold">
              <span>
                {t("staff_reports.payroll.total", {
                  n: format.number(preview.payable.length),
                })}
              </span>
              <Money amount={preview.total} currency={preview.currency} />
            </p>
          ) : null}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => change(false)}>
            {t("common.close")}
          </Button>
          <Button
            type="button"
            disabled={
              !validPeriod ||
              preview.payable.length === 0 ||
              run.isPending ||
              outcome !== null
            }
            data-testid="payroll-confirm"
            onClick={() => run.mutate(period)}
          >
            {t("staff_reports.payroll.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
