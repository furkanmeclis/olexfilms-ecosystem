"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import { Textarea } from "@/components/ui/textarea";
import {
  FieldError,
  Money,
  FormSelect,
} from "@/features/accounting/components/shared";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  accountingService,
  type FinanceAccount,
} from "@/features/accounting/services/accounting.service";
import {
  isFutureDay,
  isSalaryConflict,
  paymentFormValues,
  paymentInput,
  salaryOf,
  validatePaymentForm,
  type PaymentFormValues,
} from "@/features/staff-reports/lib/staff";
import {
  STAFF_PAYMENT_TYPES,
  staffReportsKeys,
  staffReportsService,
  type StaffPayment,
  type StaffPaymentCreateInput,
  type StaffPaymentType,
  type StaffProfile,
} from "@/features/staff-reports/services/staff-reports.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * "Add payment" (POST /v1/staff-profiles/{uuid}/payments): type, period,
 * amount (salary defaults to the card salary), the paying cash / bank
 * account, payment day and notes. A second salary for the same period is
 * 409 STAFF_SALARY_EXISTS and shows as a field error on the period.
 */
export function StaffPaymentForm({
  staff,
  accounts,
  initialType = "salary",
  pending,
  onCancel,
  onSubmit,
}: {
  staff: StaffProfile;
  accounts: FinanceAccount[];
  initialType?: StaffPaymentType;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (input: StaffPaymentCreateInput) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const active = accounts.filter((a) => a.active);
  const [values, setValues] = useState<PaymentFormValues>(() => ({
    ...paymentFormValues(initialType),
    account_uuid: active.length === 1 ? active[0]!.uuid : "",
  }));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const set = <K extends keyof PaymentFormValues>(
    key: K,
    value: PaymentFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));
  const salary = salaryOf(staff);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const next = validatePaymentForm(values, staff, t);
    setErrors(next);
    if (Object.keys(next).length) return;
    try {
      await onSubmit(paymentInput(values));
    } catch (error) {
      if (isSalaryConflict(error)) {
        setErrors({ period: t("staff_reports.payment.salary_exists") });
      } else if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
    }
  };

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="staff-payment-form"
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <FormSelect
          id="staff-payment-type"
          name="type"
          label={t("staff_reports.fields.payment_type")}
          value={values.type}
          onChange={(v) => set("type", v as StaffPaymentType)}
          options={STAFF_PAYMENT_TYPES.map((type) => ({
            value: type,
            label: t(`staff_reports.payment_types.${type}`),
          }))}
        />
        <div className="grid gap-1.5">
          <Label htmlFor="staff-payment-period">
            {t("staff_reports.fields.period")}
          </Label>
          <MonthPicker
            id="staff-payment-period"
            name="period"
            value={values.period}
            clearable
            aria-invalid={errors.period ? true : undefined}
            aria-describedby={
              errors.period ? "staff-payment-period-error" : undefined
            }
            onChange={(value) => set("period", value)}
          />
          <FieldError id="staff-payment-period-error" message={errors.period} />
        </div>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="staff-payment-amount">
          {t("staff_reports.fields.amount")}
        </Label>
        <Input
          id="staff-payment-amount"
          name="amount"
          inputMode="decimal"
          dir="ltr"
          autoComplete="off"
          className="text-end tabular-nums"
          value={values.amount}
          placeholder={
            values.type === "salary" && salary > 0 ? staff.monthly_salary! : ""
          }
          aria-invalid={errors.amount ? true : undefined}
          aria-describedby={
            errors.amount ? "staff-payment-amount-error" : undefined
          }
          onChange={(e) => set("amount", e.target.value)}
        />
        {values.type === "salary" && salary > 0 ? (
          <p className="text-muted-foreground text-xs">
            {t("staff_reports.payment.salary_default")}{" "}
            <Money amount={salary} currency={staff.currency} />
          </p>
        ) : null}
        <FieldError id="staff-payment-amount-error" message={errors.amount} />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <FormSelect
          id="staff-payment-account"
          name="account_uuid"
          label={t("staff_reports.fields.account")}
          value={values.account_uuid}
          placeholder={t("staff_reports.payment.pick_account")}
          error={errors.account_uuid}
          onChange={(v) => set("account_uuid", v)}
          options={active.map((a) => ({
            value: a.uuid,
            label: `${a.name} (${a.currency})`,
          }))}
        />
        <div className="grid gap-1.5">
          <Label htmlFor="staff-payment-paid-on">
            {t("staff_reports.fields.paid_on")}
          </Label>
          <DatePicker
            id="staff-payment-paid-on"
            value={values.paid_on}
            aria-invalid={errors.paid_on ? true : undefined}
            onChange={(v) => set("paid_on", v)}
          />
          <FieldError
            id="staff-payment-paid-on-error"
            message={errors.paid_on}
          />
          {isFutureDay(values.paid_on) ? (
            <p
              className="text-muted-foreground text-xs"
              data-testid="staff-payment-planned-hint"
            >
              {t("staff_reports.planned.future_hint")}
            </p>
          ) : null}
        </div>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="staff-payment-description">
          {t("staff_reports.fields.description")}
        </Label>
        <Textarea
          id="staff-payment-description"
          name="description"
          rows={2}
          maxLength={1500}
          value={values.description}
          onChange={(e) => set("description", e.target.value)}
        />
        <FieldError
          id="staff-payment-description-error"
          message={errors.description}
        />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="staff-payment-target-note">
          {t("staff_reports.fields.target_note")}
        </Label>
        <Input
          id="staff-payment-target-note"
          name="target_note"
          maxLength={400}
          value={values.target_note}
          onChange={(e) => set("target_note", e.target.value)}
        />
        <FieldError
          id="staff-payment-target-note-error"
          message={errors.target_note}
        />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={pending}
          data-testid="staff-payment-submit"
        >
          {t("staff_reports.payment.submit")}
        </Button>
      </DialogFooter>
    </form>
  );
}

/** Dialog wrapper: loads the active cash / bank accounts and books it. */
export function StaffPaymentDialog({
  orgUuid,
  staff,
  onOpenChange,
  onSaved,
}: {
  orgUuid: string;
  staff: StaffProfile | null;
  onOpenChange: (open: boolean) => void;
  onSaved?: (payment: StaffPayment) => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const open = Boolean(staff);

  const accounts = useQuery({
    queryKey: accountingKeys.accounts(orgUuid, { active: true }),
    queryFn: () => accountingService.listAccounts({ active: true }),
    enabled: open && Boolean(orgUuid),
  });

  const save = useMutation({
    mutationFn: (input: StaffPaymentCreateInput) =>
      staffReportsService.createPayment(staff!.uuid, input),
    onSuccess: async (payment) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: staffReportsKeys.all(orgUuid),
        }),
        queryClient.invalidateQueries({
          queryKey: accountingKeys.all(orgUuid),
        }),
      ]);
      appToast.success(
        t(
          payment.status === "planned"
            ? "staff_reports.payment.done_planned"
            : "staff_reports.payment.done",
        ),
      );
      onSaved?.(payment);
      onOpenChange(false);
    },
    onError: (error: unknown) => {
      appToast.error(
        isSalaryConflict(error)
          ? t("staff_reports.payment.salary_exists")
          : isApiError(error)
            ? error.message
            : t("accounting.toast.failed"),
      );
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("staff_reports.payment.title")}</DialogTitle>
          <DialogDescription>
            {staff ? staff.name : t("staff_reports.payment.description")}
          </DialogDescription>
        </DialogHeader>
        {!staff ? null : accounts.isLoading ? (
          <p className="text-muted-foreground text-sm">
            {t("accounting.loading")}
          </p>
        ) : (
          <StaffPaymentForm
            key={staff.uuid}
            staff={staff}
            accounts={accounts.data?.items ?? []}
            pending={save.isPending}
            onCancel={() => onOpenChange(false)}
            onSubmit={(input) => save.mutateAsync(input)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
