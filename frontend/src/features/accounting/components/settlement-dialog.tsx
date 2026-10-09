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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  FieldError,
  FormSelect,
  newIdempotencyKey,
} from "@/features/accounting/components/shared";
import { accountingKeys } from "@/features/accounting/hooks/use-accounting-access";
import {
  fieldErrors,
  settlementFormSchema,
  settlementInput,
  type SettlementFormValues,
} from "@/features/accounting/lib/form";
import {
  accountingService,
  type CariAccount,
  type FinanceAccount,
  type FinanceSettlementInput,
  type SettlementKind,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type SettlementFormProps = {
  kind: SettlementKind;
  accounts: FinanceAccount[];
  cari: CariAccount[];
  currencies: string[];
  /** Pre-selected cari (cari detail page); the select is then locked. */
  fixedCari?: CariAccount | null;
  idempotencyKey: string;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (input: FinanceSettlementInput) => Promise<unknown>;
};

/**
 * Collection (the counterparty paid us) or payment (we paid it): a cash or
 * bank account, a cari, an amount and its currency. Another currency than
 * the book's is converted at today's rate and frozen (K7). A sensitive
 * write answering STEP_UP_REQUIRED opens the step-up dialog and retries
 * (platformRequest → withStepUpRetry); the idempotency key makes the
 * retry safe.
 */
export function SettlementForm({
  kind,
  accounts,
  cari,
  currencies,
  fixedCari,
  idempotencyKey,
  pending,
  onCancel,
  onSubmit,
}: SettlementFormProps) {
  const { t } = useLocale();
  const active = accounts.filter((a) => a.active);
  const [values, setValues] = useState<SettlementFormValues>(() => ({
    account_uuid: active.length === 1 ? active[0]!.uuid : "",
    cari_uuid: fixedCari?.uuid ?? "",
    amount: "",
    currency: active.length === 1 ? active[0]!.currency : "",
    description: "",
  }));
  const [errors, setErrors] = useState<Record<string, string>>({});

  const set = <K extends keyof SettlementFormValues>(
    key: K,
    value: SettlementFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const cariOptions = (fixedCari ? [fixedCari] : cari).map((c) => ({
    value: c.uuid,
    label: c.counterparty.name,
  }));
  const currencyOptions = Array.from(
    new Set(
      [values.currency, ...active.map((a) => a.currency), ...currencies].filter(
        Boolean,
      ),
    ),
  ).map((code) => ({ value: code, label: code }));
  const account = active.find((a) => a.uuid === values.account_uuid);
  const foreign =
    Boolean(account) &&
    Boolean(values.currency) &&
    account?.currency !== values.currency;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const parsed = settlementFormSchema(t).safeParse(values);
    if (!parsed.success) {
      setErrors(fieldErrors(parsed.error));
      return;
    }
    setErrors({});
    try {
      await onSubmit(settlementInput(parsed.data, idempotencyKey));
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
      data-testid="settlement-form"
      data-kind={kind}
    >
      <FormSelect
        id="settlement-account"
        name="account_uuid"
        label={t("accounting.fields.account")}
        value={values.account_uuid}
        placeholder={t("accounting.settlement.pick_account")}
        error={errors.account_uuid}
        onChange={(v) => {
          const picked = active.find((a) => a.uuid === v);
          setValues((prev) => ({
            ...prev,
            account_uuid: v,
            currency: prev.currency || picked?.currency || "",
          }));
        }}
        options={active.map((a) => ({
          value: a.uuid,
          label: `${a.name} (${a.currency})`,
        }))}
      />
      <FormSelect
        id="settlement-cari"
        name="cari_uuid"
        label={t("accounting.fields.cari")}
        value={values.cari_uuid}
        disabled={Boolean(fixedCari)}
        placeholder={t("accounting.settlement.pick_cari")}
        error={errors.cari_uuid}
        onChange={(v) => set("cari_uuid", v)}
        options={cariOptions}
      />
      <div className="grid gap-4 sm:grid-cols-[1fr_8rem]">
        <div className="grid gap-1.5">
          <Label htmlFor="settlement-amount">
            {t("accounting.fields.amount")}
          </Label>
          <Input
            id="settlement-amount"
            name="amount"
            inputMode="decimal"
            dir="ltr"
            autoComplete="off"
            className="text-end tabular-nums"
            value={values.amount}
            aria-invalid={errors.amount ? true : undefined}
            aria-describedby={
              errors.amount ? "settlement-amount-error" : undefined
            }
            onChange={(e) => set("amount", e.target.value)}
          />
          <FieldError id="settlement-amount-error" message={errors.amount} />
        </div>
        <FormSelect
          id="settlement-currency"
          name="currency"
          label={t("accounting.fields.currency")}
          value={values.currency}
          placeholder="—"
          error={errors.currency}
          onChange={(v) => set("currency", v)}
          options={currencyOptions}
        />
      </div>
      {foreign ? (
        <p
          className="text-muted-foreground text-xs"
          data-testid="settlement-fx-hint"
        >
          {t("accounting.settlement.fx_hint", {
            from: values.currency,
            to: account?.currency ?? "",
          })}
        </p>
      ) : null}
      <div className="grid gap-1.5">
        <Label htmlFor="settlement-description">
          {t("accounting.fields.description")}
        </Label>
        <Textarea
          id="settlement-description"
          name="description"
          rows={3}
          maxLength={1000}
          value={values.description}
          placeholder={t("accounting.settlement.description_hint")}
          aria-invalid={errors.description ? true : undefined}
          onChange={(e) => set("description", e.target.value)}
        />
        <FieldError
          id="settlement-description-error"
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
          data-testid="settlement-submit"
        >
          {t(`accounting.settlement.submit_${kind}`)}
        </Button>
      </DialogFooter>
    </form>
  );
}

/**
 * Dialog wrapper: loads the active accounts, the cari list (unless one is
 * fixed) and the currencies, and books the settlement in the active book.
 */
export function SettlementDialog({
  orgUuid,
  kind,
  open,
  fixedCari,
  onOpenChange,
  onDone,
}: {
  orgUuid: string;
  kind: SettlementKind;
  open: boolean;
  fixedCari?: CariAccount | null;
  onOpenChange: (open: boolean) => void;
  onDone?: () => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  // One key per opened dialog: a retried submit (network, step-up) books once.
  const [key, setKey] = useState(newIdempotencyKey);

  const accounts = useQuery({
    queryKey: accountingKeys.accounts(orgUuid, { active: true }),
    queryFn: () => accountingService.listAccounts({ active: true }),
    enabled: open && Boolean(orgUuid),
  });
  const cari = useQuery({
    queryKey: accountingKeys.cariList(orgUuid, { active: true, limit: 100 }),
    queryFn: () => accountingService.listCari({ active: true, limit: 100 }),
    enabled: open && Boolean(orgUuid) && !fixedCari,
  });
  const currencies = useQuery({
    queryKey: accountingKeys.currencies,
    queryFn: () => accountingService.listCurrencies(),
    enabled: open,
    staleTime: 60 * 60 * 1000,
  });

  const save = useMutation({
    mutationFn: (input: FinanceSettlementInput) =>
      accountingService.settle(kind, input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(orgUuid),
      });
      appToast.success(t(`accounting.settlement.done_${kind}`));
      setKey(newIdempotencyKey());
      onOpenChange(false);
      onDone?.();
    },
    onError: (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      );
    },
  });

  const loading =
    accounts.isLoading ||
    (!fixedCari && cari.isLoading) ||
    currencies.isLoading;
  const noAccounts =
    !accounts.isLoading &&
    (accounts.data?.items ?? []).filter((a) => a.active).length === 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t(`accounting.settlement.title_${kind}`)}</DialogTitle>
          <DialogDescription>
            {t(`accounting.settlement.description_${kind}`)}
          </DialogDescription>
        </DialogHeader>
        {!open ? null : loading ? (
          <p className="text-muted-foreground text-sm">
            {t("accounting.loading")}
          </p>
        ) : noAccounts ? (
          <p className="text-muted-foreground text-sm">
            {t("accounting.settlement.no_accounts")}
          </p>
        ) : (
          <SettlementForm
            key={key}
            kind={kind}
            accounts={accounts.data?.items ?? []}
            cari={cari.data?.items ?? []}
            currencies={(currencies.data?.items ?? []).map((c) => c.code)}
            fixedCari={fixedCari}
            idempotencyKey={key}
            pending={save.isPending}
            onCancel={() => onOpenChange(false)}
            onSubmit={(input) => save.mutateAsync(input)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
