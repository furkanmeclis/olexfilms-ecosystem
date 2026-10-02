"use client";

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
import { Switch } from "@/components/ui/switch";
import {
  FieldError,
  NativeSelect,
} from "@/features/accounting/components/shared";
import {
  accountDefaults,
  accountFormSchema,
  fieldErrors,
  type AccountFormValues,
} from "@/features/accounting/lib/form";
import {
  ACCOUNT_TYPES,
  type AccountType,
  type FinanceAccount,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

type AccountFormProps = {
  account: FinanceAccount | null;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (values: AccountFormValues) => Promise<unknown>;
};

/**
 * Cash / bank account form. The type and currency are fixed once the
 * account exists (the currency is the organization's, K7); accounts are
 * never deleted, only deactivated.
 */
export function AccountForm({
  account,
  pending,
  onCancel,
  onSubmit,
}: AccountFormProps) {
  const { t } = useLocale();
  const [values, setValues] = useState<AccountFormValues>(() =>
    accountDefaults(account),
  );
  const [errors, setErrors] = useState<Record<string, string>>({});

  const set = <K extends keyof AccountFormValues>(
    key: K,
    value: AccountFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const parsed = accountFormSchema(t).safeParse(values);
    if (!parsed.success) {
      setErrors(fieldErrors(parsed.error));
      return;
    }
    setErrors({});
    try {
      await onSubmit(parsed.data);
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
      data-testid="account-form"
    >
      <NativeSelect
        id="account-type"
        name="type"
        label={t("accounting.fields.type")}
        value={values.type}
        disabled={Boolean(account)}
        onChange={(v) => set("type", v as AccountType)}
        options={ACCOUNT_TYPES.map((type) => ({
          value: type,
          label: t(`accounting.account_types.${type}`),
        }))}
      />
      <div className="grid gap-1.5">
        <Label htmlFor="account-name">{t("accounting.fields.name")}</Label>
        <Input
          id="account-name"
          name="name"
          value={values.name}
          maxLength={200}
          aria-invalid={errors.name ? true : undefined}
          aria-describedby={errors.name ? "account-name-error" : undefined}
          onChange={(e) => set("name", e.target.value)}
        />
        <FieldError id="account-name-error" message={errors.name} />
      </div>
      {values.type === "bank" ? (
        <div className="grid gap-1.5">
          <Label htmlFor="account-iban">{t("accounting.fields.iban")}</Label>
          <Input
            id="account-iban"
            name="iban"
            dir="ltr"
            autoComplete="off"
            value={values.iban}
            placeholder="TR00 0000 0000 0000 0000 0000 00"
            aria-invalid={errors.iban ? true : undefined}
            aria-describedby={errors.iban ? "account-iban-error" : undefined}
            onChange={(e) => set("iban", e.target.value)}
          />
          <FieldError id="account-iban-error" message={errors.iban} />
        </div>
      ) : null}
      {account ? (
        <div className="flex items-center justify-between gap-4">
          <div>
            <Label htmlFor="account-active">
              {t("accounting.fields.active")}
            </Label>
            <p className="text-muted-foreground text-xs">
              {t("accounting.accounts.inactive_hint")}
            </p>
          </div>
          <Switch
            id="account-active"
            checked={values.active}
            onCheckedChange={(v) => set("active", v)}
          />
        </div>
      ) : (
        <p className="text-muted-foreground text-xs">
          {t("accounting.accounts.currency_hint")}
        </p>
      )}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={pending}
          data-testid="account-form-submit"
        >
          {t("common.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function AccountFormDialog({
  open,
  account,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  account: FinanceAccount | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: AccountFormValues) => Promise<unknown>;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {account
              ? t("accounting.accounts.edit_title")
              : t("accounting.accounts.create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("accounting.accounts.form_description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <AccountForm
            key={account?.uuid ?? "new"}
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
