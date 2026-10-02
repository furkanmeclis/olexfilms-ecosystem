"use client";

import { useState, type FormEvent } from "react";

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
import { FieldError } from "@/features/accounting/components/shared";
import { normalizeAmount } from "@/features/accounting/lib/form";
import type {
  FinanceAccount,
  FinanceAccountOpeningInput,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Cash / bank account opening balance (TEC-198): a one-off amount in the
 * account's currency at a past date. It moves the account balance only
 * (no income/expense); the step-up prompt comes from platformRequest.
 */
export function AccountOpeningDialog({
  account,
  pending,
  onOpenChange,
  onSubmit,
}: {
  account: FinanceAccount | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: FinanceAccountOpeningInput) => Promise<unknown>;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={Boolean(account)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("accounting.opening.title")}</DialogTitle>
          <DialogDescription>
            {t("accounting.opening.description")}
          </DialogDescription>
        </DialogHeader>
        {account ? (
          <AccountOpeningForm
            key={account.uuid}
            account={account}
            pending={pending}
            onCancel={() => onOpenChange(false)}
            onSubmit={onSubmit}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

export function AccountOpeningForm({
  account,
  pending,
  onCancel,
  onSubmit,
}: {
  account: FinanceAccount;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (body: FinanceAccountOpeningInput) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [amount, setAmount] = useState("");
  const [date, setDate] = useState("");
  const [description, setDescription] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const next: Record<string, string> = {};
    const value = normalizeAmount(amount);
    if (!value) next.amount = t("accounting.validation.amount");
    if (!DATE_RE.test(date))
      next.opening_date = t("accounting.validation.date");
    if (description.length > 1000) {
      next.description = t("accounting.validation.too_long", { max: 1000 });
    }
    setErrors(next);
    if (!value || Object.keys(next).length > 0) return;
    try {
      await onSubmit({
        amount: value,
        opening_date: date,
        description: description.trim() || undefined,
      });
    } catch (error) {
      if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
    }
  };

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="account-opening-form"
    >
      <div className="grid gap-1.5">
        <Label htmlFor="opening-amount">
          {t("accounting.fields.amount")} ({account.currency})
        </Label>
        <Input
          id="opening-amount"
          name="amount"
          inputMode="decimal"
          dir="ltr"
          value={amount}
          aria-invalid={errors.amount ? true : undefined}
          aria-describedby={errors.amount ? "opening-amount-error" : undefined}
          onChange={(e) => setAmount(e.target.value)}
        />
        <FieldError id="opening-amount-error" message={errors.amount} />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="opening-date">
          {t("accounting.fields.opening_date")}
        </Label>
        <DatePicker
          id="opening-date"
          value={date}
          aria-invalid={errors.opening_date ? true : undefined}
          onChange={(v) => setDate(v)}
        />
        <FieldError id="opening-date-error" message={errors.opening_date} />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="opening-description">
          {t("accounting.fields.description")}
        </Label>
        <Input
          id="opening-description"
          name="description"
          maxLength={1000}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <FieldError
          id="opening-description-error"
          message={errors.description}
        />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={pending}
          data-testid="account-opening-submit"
        >
          {t("common.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}
